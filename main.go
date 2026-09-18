package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}
*/
import "C"

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const abiVersion uint32 = 1

const (
	pluginID      = "skin-center"
	pluginVersion = "1.8.0"
	resourcePath  = "/skin"
	// assetRelPath and injectRelPath are resolved against the server working
	// directory; both ship next to the library in the plugins dir.
	assetRelPath   = "plugins/skin-center.html"
	injectRelPath  = "plugins/skin-center-inject.html"
	skinMarkStart  = "<!--cpa-skin-center-start-->"
	skinMarkEnd    = "<!--cpa-skin-center-end-->"
	panelAssetName = "management.html"
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(abiVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

type pluginRequest struct {
	Path string `json:"Path"`
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	raw, errHandle := handleMethod(C.GoString(method), C.GoBytes(unsafe.Pointer(request), C.int(requestLen)))
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, length C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
	_ = length
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func skinDebug(format string, args ...any) {
	if os.Getenv("SKIN_CENTER_DEBUG") == "" {
		return
	}
	f, err := os.OpenFile("skin-center-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	msg := fmt.Sprintf(time.Now().Format("15:04:05.000 ")+format+"\n", args...)
	f.WriteString(msg)
}

func handleMethod(method string, requestBody []byte) ([]byte, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		skinDebug("register lifecycle method=%s", method)
		extractSiteConfig(requestBody)
		startPanelSkinner()
		return okEnvelopeJSON(registerResponse)
	case "management.register":
		return okEnvelopeJSON(managementRegisterResponse)
	case "management.handle":
		var req pluginRequest
		if len(requestBody) > 0 {
			if errUnmarshal := json.Unmarshal(requestBody, &req); errUnmarshal != nil {
				return nil, errUnmarshal
			}
		}
		return managementHandle(req.Path)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

// startPanelSkinner keeps the local management panel asset skinned. The host
// re-downloads management.html from upstream releases on its own schedule;
// the loop re-applies the injection after every upstream refresh, and strips
// it again when the external inject asset is removed.
func startPanelSkinner() {
	skinnerOnce.Do(func() {
		go func() {
			skinDebug("skinner goroutine started")
			ensurePanelSkin()
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				ensurePanelSkin()
			}
		}()
	})
}

var skinnerOnce sync.Once

func panelAssetCandidates() []string {
	var out []string
	add := func(p string) {
		if p != "" {
			out = append(out, p)
		}
	}
	if override := strings.TrimSpace(os.Getenv("MANAGEMENT_STATIC_PATH")); override != "" {
		if strings.EqualFold(filepath.Base(override), panelAssetName) {
			add(override)
		} else {
			add(filepath.Join(override, panelAssetName))
		}
	}
	if writable := strings.TrimSpace(os.Getenv("WRITABLE_PATH")); writable != "" {
		add(filepath.Join(writable, "static", panelAssetName))
	}
	if wd, errWd := os.Getwd(); errWd == nil {
		add(filepath.Join(wd, "static", panelAssetName))
	}
	if home, errHome := os.UserHomeDir(); errHome == nil {
		add(filepath.Join(home, ".cli-proxy-api", "static", panelAssetName))
	}
	return out
}

func injectAsset() ([]byte, bool) {
	if data, errRead := os.ReadFile(injectRelPath); errRead == nil && len(data) > 0 {
		return data, true
	}
	return []byte(skinInjectHTML), true
}

// ensurePanelSkin is idempotent: injected state on disk always converges to
// (asset present ? asset content : original file without markers).
func ensurePanelSkin() {
	block, _ := injectAsset()
	paths := panelAssetCandidates()
	skinDebug("ensurePanelSkin candidates=%v", paths)
	for _, path := range paths {
		data, errRead := os.ReadFile(path)
		if errRead != nil {
			skinDebug("read fail %s: %v", path, errRead)
			continue
		}
		skinDebug("read ok %s len=%d", path, len(data))
		next, changed := spliceSkin(string(data), string(block))
		skinDebug("splice changed=%v len=%d", changed, len(next))
		if !changed {
			continue
		}
		if errW := writePanelAsset(path, []byte(next)); errW != nil {
			skinDebug("write fail %s: %v", path, errW)
		} else {
			skinDebug("write ok %s", path)
		}
	}
}

func spliceSkin(html string, block string) (string, bool) {
	desired := skinMarkStart + "\n" + block + "\n" + skinMarkEnd
	if start := strings.Index(html, skinMarkStart); start >= 0 {
		end := strings.Index(html, skinMarkEnd)
		if end < start {
			return html, false
		}
		stop := end + len(skinMarkEnd)
		if html[start:stop] == desired {
			return html, false
		}
		html = html[:start] + html[stop:]
	}
	if head := strings.LastIndex(html, "</head>"); head >= 0 {
		return html[:head] + desired + html[head:], true
	}
	return html + desired, true
}

func writePanelAsset(path string, data []byte) error {
	tmp, errCreate := os.CreateTemp(filepath.Dir(path), "cpa-skin-*.html")
	if errCreate != nil {
		return errCreate
	}
	name := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(name)
	}()
	if _, errWrite := tmp.Write(data); errWrite != nil {
		return errWrite
	}
	if errChmod := tmp.Chmod(0o644); errChmod != nil {
		return errChmod
	}
	if errClose := tmp.Close(); errClose != nil {
		return errClose
	}
	return os.Rename(name, path)
}

const registerResponse = `{"schema_version":1,"metadata":{"Name":"` + pluginID + `","Version":"` + pluginVersion + `","Author":"Lemon","Description":"CLIProxyAPI 管理面板皮肤中心：多主题预览与应用","GitHubRepository":"https://github.com/lemon-casino/CLIProxyAPI-Skin","ConfigFields":[]},"capabilities":{"management_api":true}}`

const managementRegisterResponse = `{"resources":[{"Path":"` + resourcePath + `","Menu":"皮肤中心","Description":"CLIProxyAPI 面板皮肤中心：多主题预览、切换与收藏"},{"Path":"/config","Description":"站点级皮肤配置（只读 JSON）"}]}`

// siteConfig holds the site-wide skin config (base64-decoded from the
// site-config key inside plugins.configs.skin-center). The host pushes the
// subtree via plugin.register/plugin.reconfigure on every config.yaml change,
// so config.yaml is the persistence layer and this value is just a mirror.
var (
	siteConfigMu   sync.RWMutex
	siteConfigJSON = []byte(`{}`)
)

func extractSiteConfig(envelope []byte) {
	if len(envelope) == 0 {
		return
	}
	// The lifecycle request wraps the YAML as {"config_yaml":"<base64>"}.
	var wrapper struct {
		ConfigYAML []byte `json:"config_yaml"`
	}
	if errUnmarshal := json.Unmarshal(envelope, &wrapper); errUnmarshal != nil {
		return
	}
	configYAML := wrapper.ConfigYAML
	if len(configYAML) == 0 {
		return
	}
	i := bytes.Index(configYAML, []byte("site-config:"))
	if i < 0 {
		return
	}
	rest := configYAML[i+len("site-config:"):]
	// Take up to the next top-level key (a newline followed by a non-indented
	// character); YAML may fold or quote long base64 values, so strip all
	// whitespace and quotes before decoding.
	end := len(rest)
	for j := 0; j < len(rest); j++ {
		if rest[j] == '\n' && j+1 < len(rest) && rest[j+1] != ' ' && rest[j+1] != '\t' && rest[j+1] != '\n' {
			end = j
			break
		}
	}
	clean := make([]byte, 0, len(rest[:end]))
	for _, c := range rest[:end] {
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '"' || c == '\'' {
			continue
		}
		clean = append(clean, c)
	}
	if len(clean) == 0 {
		return
	}
	raw, errDecode := base64.StdEncoding.DecodeString(string(clean))
	if errDecode != nil || len(raw) == 0 {
		skinDebug("site-config decode failed: %v", errDecode)
		return
	}
	siteConfigMu.Lock()
	siteConfigJSON = raw
	siteConfigMu.Unlock()
	skinDebug("site-config updated len=%d", len(raw))
}

func siteConfig() []byte {
	siteConfigMu.RLock()
	defer siteConfigMu.RUnlock()
	return bytes.Clone(siteConfigJSON)
}

func managementHandle(path string) ([]byte, error) {
	if strings.HasSuffix(strings.TrimRight(path, "/"), "/config") {
		return okEnvelopeJSON(`{"StatusCode":200,"Headers":{"Content-Type":["application/json; charset=utf-8"]},"Body":"` + base64.StdEncoding.EncodeToString(siteConfig()) + `"}`)
	}
	if strings.HasSuffix(strings.TrimRight(path, "/"), resourcePath) {
		body, source := skinPage()
		return okEnvelopeJSON(`{"StatusCode":200,"Headers":{"Content-Type":["text/html; charset=utf-8"],"X-Skin-Source":["` + source + `"]},"Body":"` + base64.StdEncoding.EncodeToString(body) + `"}`)
	}
	return okEnvelopeJSON(`{"StatusCode":404,"Headers":{"Content-Type":["application/json"]},"Body":"` + base64.StdEncoding.EncodeToString([]byte(`{"error":"not_found"}`)) + `"}`)
}

// skinPage decouples the page asset from the compiled library: prefer an
// external file so skins can be swapped without rebuilding the plugin,
// and fall back to the page embedded at build time.
func skinPage() ([]byte, string) {
	if data, errRead := os.ReadFile(assetRelPath); errRead == nil && len(data) > 0 {
		return data, "file"
	}
	return []byte(skinPageHTML), "embedded"
}

func okEnvelopeJSON(result string) ([]byte, error) {
	return json.Marshal(envelope{OK: true, Result: json.RawMessage(result)})
}

func errorEnvelope(code string, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
