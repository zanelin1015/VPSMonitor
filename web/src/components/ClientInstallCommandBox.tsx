import { Alert, Button, Input, Typography } from 'antd'
import { CopyOutlined } from '@ant-design/icons'

const { Text } = Typography

export interface ClientInstallCommandBoxProps {
  title: string
  description: string
  command: string
  onCopy: () => void
}

export function ClientInstallCommandBox(props: ClientInstallCommandBoxProps) {
  return (
    <div>
      <Alert type="info" showIcon message={props.title} description={props.description} className="compact-alert" />
      <div className="client-install-command-title">
        <Text strong>{props.title}</Text>
        <Button size="small" icon={<CopyOutlined />} onClick={props.onCopy}>
          复制
        </Button>
      </div>
      <Input.TextArea
        className="client-install-command"
        value={props.command}
        readOnly
        autoSize={{ minRows: 5, maxRows: 9 }}
      />
    </div>
  )
}

