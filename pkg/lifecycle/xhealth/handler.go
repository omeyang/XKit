package xhealth

import (
	"encoding/json"
	"net/http"
	"strings"
)

const (
	pathLiveness  = "/healthz"
	pathReadiness = "/readyz"
	pathStartup   = "/startupz"

	responseOK    = "ok"
	responseNotOK = "not ok"
)

// registerHandlers 注册 HTTP 路由。
func (h *Health) registerHandlers(mux *http.ServeMux) {
	base := strings.TrimRight(h.opts.basePath, "/")

	mux.HandleFunc(base+pathLiveness+"/", h.makeSubPathHandler(endpointLiveness))
	mux.HandleFunc(base+pathLiveness, h.makeEndpointHandler(endpointLiveness))
	mux.HandleFunc(base+pathReadiness+"/", h.makeSubPathHandler(endpointReadiness))
	mux.HandleFunc(base+pathReadiness, h.makeEndpointHandler(endpointReadiness))
	mux.HandleFunc(base+pathStartup+"/", h.makeSubPathHandler(endpointStartup))
	mux.HandleFunc(base+pathStartup, h.makeEndpointHandler(endpointStartup))
}

// makeEndpointHandler 创建端点级别的 HTTP handler。
func (h *Health) makeEndpointHandler(ep endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := h.check(r.Context(), ep)
		h.writeResponse(w, r, result)
	}
}

// makeSubPathHandler 创建子路径查询的 HTTP handler。
func (h *Health) makeSubPathHandler(ep endpoint) http.HandlerFunc {
	base := strings.TrimRight(h.opts.basePath, "/")
	paths := [3]string{
		endpointLiveness:  base + pathLiveness + "/",
		endpointReadiness: base + pathReadiness + "/",
		endpointStartup:   base + pathStartup + "/",
	}

	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, paths[ep])
		if name == "" {
			h.makeEndpointHandler(ep)(w, r)
			return
		}

		result := h.check(r.Context(), ep)
		cr, ok := result.Checks[name]
		if !ok {
			http.NotFound(w, r)
			return
		}

		h.writeSingleCheckResponse(w, r, cr)
	}
}

// writeResponse 将聚合结果写入 HTTP 响应。
func (h *Health) writeResponse(w http.ResponseWriter, r *http.Request, result *Result) {
	if h.wantsDetail(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode(result.Status))
		writeJSON(w, result)
		return
	}

	w.WriteHeader(statusCode(result.Status))
	if result.Status.IsHealthy() {
		writeBody(w, []byte(responseOK))
	} else {
		writeBody(w, []byte(responseNotOK))
	}
}

// writeSingleCheckResponse 将单个检查结果写入 HTTP 响应。
func (h *Health) writeSingleCheckResponse(w http.ResponseWriter, r *http.Request, cr CheckResult) {
	if h.wantsDetail(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode(cr.Status))
		writeJSON(w, cr)
		return
	}

	w.WriteHeader(statusCode(cr.Status))
	if cr.Status == StatusUp {
		writeBody(w, []byte(responseOK))
	} else {
		writeBody(w, []byte(responseNotOK))
	}
}

// wantsDetail 判断请求是否要求详细 JSON 响应。
func (h *Health) wantsDetail(r *http.Request) bool {
	return r.URL.Query().Get(h.opts.detailQueryParam) != ""
}

// statusCode 将 Status 映射为 HTTP 状态码。
func statusCode(s Status) int {
	if s == StatusDown {
		return http.StatusServiceUnavailable
	}
	return http.StatusOK
}

// writeBody 写入 HTTP 响应体。
// 写入失败时不返回错误，因为此时连接可能已断开，无法进行补救。
func writeBody(w http.ResponseWriter, data []byte) {
	if _, err := w.Write(data); err != nil {
		return
	}
}

// writeJSON 将值编码为 JSON 写入 HTTP 响应体。
// 编码失败时不返回错误，因为此时响应头已发送，无法更改状态码。
func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return
	}
}
