package model

import (
	"fmt"
	"slices"
	"strings"
)

const AgentPurposeIXRelay = "ix_relay"
const CustomerRouteTypeIEPL = "iepl"

type AgentClassification struct {
	Purposes              []string `json:"purposes,omitempty"`
	CountryCode           string   `json:"country_code,omitempty"`
	City                  string   `json:"city,omitempty"`
	AssignmentPurposesSet bool     `json:"assignment_purposes_set,omitempty"`
}

type AgentAssignmentRoles struct {
	Line    bool `json:"line"`
	Landing bool `json:"landing"`
}

func agentPurposeRole(purpose string) string {
	switch purpose {
	case "domestic_entry", AgentPurposeIXRelay, "optimized_relay", "xray_line":
		return AgentRoleLine
	case "residential_exit", "datacenter_exit":
		return AgentRoleLanding
	default:
		return ""
	}
}

func IsLegacyAgentRoleTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	return tag == AgentRoleLine || tag == AgentRoleLanding
}

func WithoutLegacyAgentRoleTags(tags []string) []string {
	if !slices.ContainsFunc(tags, IsLegacyAgentRoleTag) {
		return tags
	}
	return slices.DeleteFunc(slices.Clone(tags), IsLegacyAgentRoleTag)
}

func (agent AgentRecord) AssignmentRoles() AgentAssignmentRoles {
	var roles AgentAssignmentRoles
	classification := agent.Config.Entry.Classification
	if classification != nil {
		for _, purpose := range classification.Purposes {
			switch agentPurposeRole(purpose) {
			case AgentRoleLine:
				roles.Line = true
			case AgentRoleLanding:
				roles.Landing = true
			}
		}
	}
	// Existing installations used role tags before purposes controlled authorization.
	if classification == nil || !classification.AssignmentPurposesSet {
		roles.Line = roles.Line || slices.Contains(agent.Tags, AgentRoleLine)
		roles.Landing = roles.Landing || slices.Contains(agent.Tags, AgentRoleLanding)
	}
	return roles
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
			if agentPurposeRole(purpose) == "" {
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
		next.AssignmentPurposesSet = true
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
	case CustomerRouteTypeIEPL, "optimized", "standard":
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
