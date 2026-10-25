package server

import (
	"fmt"
	"log"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

// Expiry notices follow the same client scope as the area console. They use
// canonical target clients, so Realm/HAProxy paths cannot duplicate a notice.
func (s *alertService) evaluateAreaClientExpiryAlerts(agents []model.AgentRecord, snapshots map[string]model.AgentSnapshot, now time.Time) {
	ownerIDs, err := s.store.ListTelegramExpiryOwnerIDs()
	if err != nil {
		log.Printf("list area telegram owners: %v", err)
		return
	}
	permissions := &App{store: s.store}
	for _, ownerID := range ownerIDs {
		manager, found, err := s.store.GetAreaManager(ownerID)
		if err != nil || !found || !manager.Enabled {
			continue
		}
		bots, err := s.store.ListEnabledTelegramBotSecrets(ownerID)
		if err != nil {
			log.Printf("load area telegram bots: %v", err)
			continue
		}
		if len(bots) == 0 {
			continue
		}
		user := model.AdminUser{ID: manager.ID, Role: model.AdminRoleAreaManager, Username: manager.Username, AgentIDs: manager.AgentIDs}
		scope := permissions.areaManagerClientScope(user)
		tagMap, err := s.store.ListAreaManagerAgentTags(ownerID)
		if err != nil {
			log.Printf("load area telegram tags: %v", err)
			continue
		}
		for _, agent := range agents {
			if _, allowed := scope.agents[agent.AgentID]; !allowed {
				continue
			}
			overview := dashboard.BuildXUIOverview(snapshots[agent.AgentID])
			if overview == nil {
				continue
			}
			clients := make([]model.XUIClientView, 0)
			for _, client := range overview.Clients {
				if scope.allowsClient(agent.AgentID, client.InboundID, client.InboundTag, client.Email) {
					clients = append(clients, client)
				}
			}
			agent.AgentName = areaManagerDisplayName(agent.CustomerDisplayName, agent.AgentName, agent.AgentID)
			agent.Tags = tagMap[agent.AgentID]
			for _, alert := range buildXUIClientExpiryAlerts(agent, clients, now) {
				// Root and every area owner have independent cooldown state.
				alert.key = fmt.Sprintf("area:%d:%s", ownerID, alert.key)
				if strings.HasSuffix(alert.key, ":resolved") {
					_ = s.store.ResolveAlert(strings.TrimSuffix(alert.key, ":resolved"))
					continue
				}
				text := formatTelegramAlert(alert)
				send, err := s.store.ShouldSendAlert(alert.key, alert.fingerprint, text, alertCooldown)
				if err != nil || !send {
					continue
				}
				for _, bot := range bots {
					if !bot.NotifyExpiry {
						continue
					}
					if err := s.sendTelegramMessage(bot.BotToken, bot.ChatID, text); err != nil {
						log.Printf("send expiry to area %d bot %d: %v", ownerID, bot.ID, err)
					}
				}
			}
		}
	}
}
