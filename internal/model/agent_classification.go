package model

import (
	"fmt"
	"slices"
	"strings"
)

const AgentPurposeIXRelay = "ix_relay"

type AgentClassification struct {
	Purposes    []string `json:"purposes,omitempty"`
	CountryCode string   `json:"country_code,omitempty"`
	City        string   `json:"city,omitempty"`
}

type AgentClassificationPatch struct {
	Purposes    *[]string `json:"purposes,omitempty"`
	PurposeMode string    `json:"purpose_mode,omitempty"`
	CountryCode *string   `json:"country_code,omitempty"`
	City        *string   `json:"city,omitempty"`
}

func (p AgentClassificationPatch) Apply(existing *AgentClassification) (*AgentClassification, error) {
	next := AgentClassification{}
	if existing != nil {
		next = *existing
	}
	if p.Purposes != nil {
		if p.PurposeMode != "" && p.PurposeMode != "replace" && p.PurposeMode != "add" && p.PurposeMode != "remove" {
			return nil, fmt.Errorf("unknown purpose change mode %q", p.PurposeMode)
		}
		for _, purpose := range *p.Purposes {
			switch purpose {
			case "domestic_entry", AgentPurposeIXRelay, "optimized_relay", "xray_line", "residential_exit", "datacenter_exit":
			default:
				return nil, fmt.Errorf("unknown Client purpose %q", purpose)
			}
		}
		switch p.PurposeMode {
		case "add":
			next.Purposes = append(slices.Clone(next.Purposes), (*p.Purposes)...)
		case "remove":
			next.Purposes = slices.DeleteFunc(slices.Clone(next.Purposes), func(purpose string) bool { return slices.Contains(*p.Purposes, purpose) })
		default:
			next.Purposes = slices.Clone(*p.Purposes)
		}
		slices.Sort(next.Purposes)
		next.Purposes = slices.Compact(next.Purposes)
	}
	if p.CountryCode != nil {
		next.CountryCode = strings.ToUpper(strings.TrimSpace(*p.CountryCode))
		code := next.CountryCode
		if code != "" && (len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z') {
			return nil, fmt.Errorf("country code must contain two letters")
		}
	}
	if p.City != nil {
		next.City = strings.TrimSpace(*p.City)
		if len([]rune(next.City)) > 80 {
			return nil, fmt.Errorf("city must be at most 80 characters")
		}
	}
	return &next, nil
}

type CustomerRouteConfig struct {
	Name       string `json:"name,omitempty"`
	Type       string `json:"type,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

func (c CustomerRouteConfig) Normalize() CustomerRouteConfig {
	c.Name = strings.TrimSpace(c.Name)
	switch c.Type {
	case "iepl", "optimized", "standard":
	default:
		c.Type = ""
	}
	switch c.Visibility {
	case "public", "internal":
	default:
		c.Visibility = ""
	}
	return c
}
