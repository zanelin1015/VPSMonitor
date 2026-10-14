import type { ScheduledTaskSettings } from '../types'

export function defaultScheduledTaskSettings(): ScheduledTaskSettings {
  return {
    alert_sweep: { enabled: true, interval_minutes: 5 },
    daily_traffic_report: { enabled: true, time_of_day: '09:00', interval_days: 1 },
  }
}

export function normalizeScheduledTaskSettings(settings?: ScheduledTaskSettings): ScheduledTaskSettings {
  const defaults = defaultScheduledTaskSettings()
  return {
    alert_sweep: {
      enabled: settings?.alert_sweep?.enabled ?? defaults.alert_sweep.enabled,
      interval_minutes: Math.min(1440, Math.max(1, Number(settings?.alert_sweep?.interval_minutes || defaults.alert_sweep.interval_minutes || 5))),
    },
    daily_traffic_report: {
      enabled: settings?.daily_traffic_report?.enabled ?? defaults.daily_traffic_report.enabled,
      time_of_day: /^\d{2}:\d{2}$/.test(settings?.daily_traffic_report?.time_of_day || '') ? settings?.daily_traffic_report?.time_of_day : defaults.daily_traffic_report.time_of_day,
      interval_days: Math.min(365, Math.max(1, Number(settings?.daily_traffic_report?.interval_days || defaults.daily_traffic_report.interval_days || 1))),
    },
  }
}
