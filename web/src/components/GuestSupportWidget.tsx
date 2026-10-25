import { SupportChatWidget } from './SupportChatWidget'

/** An area URL change starts a separate guest UI and never reuses the previous conversation. */
export function GuestSupportWidget({ scopeToken }: { scopeToken?: string }) {
  return <SupportChatWidget key={`guest:${scopeToken || ''}`} identity="guest" scopeToken={scopeToken} />
}
