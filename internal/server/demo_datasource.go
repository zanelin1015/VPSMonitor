package server

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func newDemoDataSource(rawURL string) (http.Handler, error) {
	target, err := url.Parse(strings.TrimRight(strings.TrimSpace(rawURL), "/"))
	if err != nil {
		return nil, fmt.Errorf("parse demo data source url: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("demo data source url must start with http:// or https://")
	}
	targetOrigin := target.Scheme + "://" + target.Host
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Host = target.Host
		if r.Header.Get("Origin") != "" {
			r.Header.Set("Origin", targetOrigin)
		}
		if referer := r.Header.Get("Referer"); referer != "" {
			r.Header.Set("Referer", targetOrigin+"/")
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		rewriteDemoDataSourceCookies(resp)
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		writeError(w, http.StatusBadGateway, "demo data source request failed: "+err.Error())
	}
	return proxy, nil
}

func rewriteDemoDataSourceCookies(resp *http.Response) {
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return
	}
	resp.Header.Del("Set-Cookie")
	for _, cookie := range cookies {
		cookie.Domain = ""
		cookie.Path = "/"
		cookie.Secure = false
		cookie.SameSite = http.SameSiteLaxMode
		resp.Header.Add("Set-Cookie", cookie.String())
	}
}
