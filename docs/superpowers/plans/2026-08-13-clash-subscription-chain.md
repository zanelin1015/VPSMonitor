# Clash Subscription Chain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the generated Clash/Mihomo subscription support one fixed chained path where the exit node `999` is dialed through the front node `IEPL`, while keeping the rest of the subscription unchanged.

**Architecture:** Extend the Mihomo YAML writer to emit an optional `dialer-proxy` field on a specific proxy entry, then wire that field only for the fixed exit node in the subscription generator. Keep the existing proxy list, proxy-groups, and rules untouched so the change is isolated to chain routing.

**Tech Stack:** Go, `go test`, existing YAML string rendering helpers, Mihomo subscription template.

---

### Task 1: Add a failing test for the fixed chain

**Files:**
- Modify: `internal/server/customer_subscription_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestBuildMihomoSubscriptionChainsExitNodeThroughFrontNode(t *testing.T) {
	user := model.CustomerUser{Username: "alice"}
	links := []model.CustomerLinkView{
		{EntryClientName: "IEPL", Remark: "IEPL", ImportURL: "ss://YWVzLTI1Ni1nY206cGFzcw@example.com:8388#IEPL", Resolved: true},
		{EntryClientName: "999", Remark: "999", ImportURL: "vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=none#999", Resolved: true},
	}

	content := buildMihomoSubscription(user, links)
	assertContains(t, content, `name: "IEPL"`)
	assertContains(t, content, `name: "999"`)
	assertContains(t, content, `dialer-proxy: "IEPL"`)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/server -run TestBuildMihomoSubscriptionChainsExitNodeThroughFrontNode -v`
Expected: FAIL because `dialer-proxy` is not rendered yet.

- [ ] **Step 3: Keep the test as the behavior contract**

Do not change the assertion to fit the current code.

### Task 2: Render `dialer-proxy` in Mihomo proxy output

**Files:**
- Modify: `internal/server/customer_subscription.go`

- [ ] **Step 1: Extend the proxy model and writer**

```go
type mihomoProxy struct {
	Name         string
	DialerProxy  string
	Fields       []mihomoField
	Objects      []mihomoObject
}

func (p mihomoProxy) writeYAML(b *strings.Builder) {
	b.WriteString("  - name: ")
	b.WriteString(yamlString(p.Name))
	b.WriteString("\n")
	if p.DialerProxy != "" {
		b.WriteString("    dialer-proxy: ")
		b.WriteString(yamlString(p.DialerProxy))
		b.WriteString("\n")
	}
	...
}
```

- [ ] **Step 2: Add a fixed chain rule when building proxies**

```go
const (
	customerSubscriptionChainFrontName = "IEPL"
	customerSubscriptionChainExitName  = "999"
)

func buildMihomoSubscription(user model.CustomerUser, links []model.CustomerLinkView) string {
	...
	for i := range proxies {
		if proxies[i].Name == customerSubscriptionChainExitName {
			proxies[i].DialerProxy = customerSubscriptionChainFrontName
		}
	}
	...
}
```

- [ ] **Step 3: Run the focused test again**

Run: `go test ./internal/server -run TestBuildMihomoSubscriptionChainsExitNodeThroughFrontNode -v`
Expected: PASS and the YAML output contains `dialer-proxy: "IEPL"` on the `999` node.

### Task 3: Verify the full subscription output still stays intact

**Files:**
- Modify: `internal/server/customer_subscription_test.go`

- [ ] **Step 1: Extend the existing subscription test**

```go
assertContains(t, content, `name: 🚀 节点选择`)
assertContains(t, content, `name: ♻️ 自动选择`)
assertContains(t, content, `rules:`)
assertContains(t, content, `dialer-proxy: "IEPL"`)
```

- [ ] **Step 2: Run the package tests**

Run: `go test ./internal/server -run TestBuildMihomoSubscription -v`
Expected: PASS with no changes to existing groups or rules.

### Task 4: Produce a downloadable final file

**Files:**
- Add: `artifacts/customer-subscription-chain-example.yaml`

- [ ] **Step 1: Render a sample YAML artifact from the updated generator**

Use the test fixture or a small helper to write the generated subscription into the artifact file so it can be downloaded directly.

- [ ] **Step 2: Confirm the artifact contains the chain**

Check that the file includes both `IEPL`, `999`, and `dialer-proxy: "IEPL"`.

- [ ] **Step 3: Commit the finished change**

```bash
git add internal/server/customer_subscription.go internal/server/customer_subscription_test.go artifacts/customer-subscription-chain-example.yaml docs/superpowers/plans/2026-08-13-clash-subscription-chain.md
git commit -m "feat: chain Clash subscription exit through IEPL"
```

---

### Self-Review

- The plan covers the renderer, the subscription generator, the tests, and the downloadable artifact.
- No placeholders remain.
- The chain is fixed and isolated to the `999 -> IEPL` relationship, so future chain files can change the constants without redesigning the generator.
