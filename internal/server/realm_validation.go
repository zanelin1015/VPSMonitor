package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"bridge-core/internal/model"
)

var realmForwardLookupHost = func(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

func validateRealmForwardTargets(ctx context.Context, cfg model.RealmForwardConfig) error {
	if !cfg.Enabled || strings.EqualFold(strings.TrimSpace(cfg.Backend), "none") {
		return nil
	}
	for _, rule := range cfg.Rules {
		if !rule.Enabled {
			continue
		}
		label := strings.TrimSpace(rule.Name)
		if label == "" {
			label = fmt.Sprintf("监听端口 %d", rule.ListenPort)
		}
		if rule.ListenPort <= 0 || rule.ListenPort > 65535 {
			return fmt.Errorf("Realm 转发 %s 的监听端口无效", label)
		}
		if rule.TargetPort <= 0 || rule.TargetPort > 65535 {
			return fmt.Errorf("Realm 转发 %s 的目标端口无效", label)
		}
		if err := validateRealmForwardTargetHost(ctx, label, rule.TargetAddress); err != nil {
			return err
		}
	}
	return nil
}

func validateRealmForwardTargetHost(ctx context.Context, label string, rawHost string) error {
	host := strings.TrimSpace(rawHost)
	if host == "" {
		return fmt.Errorf("Realm 转发 %s 的目标地址不能为空", label)
	}
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/?#") {
		return fmt.Errorf("Realm 转发 %s 的目标地址只填写域名或 IP，不要包含协议、路径或参数", label)
	}
	if strings.ContainsAny(host, " \t\r\n") {
		return fmt.Errorf("Realm 转发 %s 的目标地址包含空白字符", label)
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.Trim(host, "[]")
	}
	if ip := net.ParseIP(host); ip != nil {
		return nil
	}
	if strings.Contains(host, ":") {
		return fmt.Errorf("Realm 转发 %s 的目标地址不要包含端口，请把端口填写到目标端口字段", label)
	}
	if !strings.Contains(host, ".") {
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ips, err := realmForwardLookupHost(lookupCtx, host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("Realm 转发 %s 的目标地址 %q 无法解析，请检查域名拼写或 DNS 记录", label, host)
	}
	return nil
}
