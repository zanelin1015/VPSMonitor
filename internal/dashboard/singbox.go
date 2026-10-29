package dashboard

import "bridge-core/internal/model"

func BuildSingBoxOverview(snapshot model.AgentSnapshot) *model.SingBoxOverview {
	if snapshot.SingBox == nil {
		return nil
	}
	return &model.SingBoxOverview{
		AgentID:         snapshot.AgentID,
		AgentName:       snapshot.AgentName,
		ConfigPath:      snapshot.SingBox.ConfigPath,
		ServiceName:     snapshot.SingBox.ServiceName,
		BinaryPath:      snapshot.SingBox.BinaryPath,
		Running:         snapshot.SingBox.Running,
		ReportedAt:      snapshot.ReportedAt,
		CollectedAt:     snapshot.SingBox.CollectedAt,
		Nodes:           snapshot.SingBox.Nodes,
		Clients:         snapshot.SingBox.Clients,
		ForwardingRules: snapshot.SingBox.ForwardingRules,
		RawConfig:       snapshot.SingBox.RawConfig,
		Error:           snapshot.SingBox.Error,
	}
}
