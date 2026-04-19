package middleware

import (
	"net/http"
	"strings"
)

type CORSConfig struct {
	AllowedOrigins []string
}

func CORS(config CORSConfig) func(http.Handler) http.Handler {
	allowedOrigins := make(map[string]struct{}, len(config.AllowedOrigins))
	for _, origin := range config.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		allowedOrigins[origin] = struct{}{}
	}
	if len(allowedOrigins) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			if _, ok := allowedOrigins[origin]; !ok {
				next.ServeHTTP(w, r)
				return
			}

			addVaryHeader(w.Header(), "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)

			if r.Method == http.MethodOptions {
				requestMethod := strings.TrimSpace(r.Header.Get("Access-Control-Request-Method"))
				if requestMethod == "" {
					next.ServeHTTP(w, r)
					return
				}

				addVaryHeader(w.Header(), "Access-Control-Request-Method")
				requestHeaders := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers"))
				if requestHeaders != "" {
					addVaryHeader(w.Header(), "Access-Control-Request-Headers")
					w.Header().Set("Access-Control-Allow-Headers", requestHeaders)
				}
				w.Header().Set("Access-Control-Allow-Methods", requestMethod)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func addVaryHeader(header http.Header, value string) {
	if header == nil {
		return
	}
	for _, existing := range header.Values("Vary") {
		for _, part := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
