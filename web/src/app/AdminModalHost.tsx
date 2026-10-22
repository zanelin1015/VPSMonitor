import { lazy, Suspense, type ComponentProps } from 'react'
const ConsoleModals = lazy(() =>
  import('../components/ConsoleModals').then((module) => ({ default: module.ConsoleModals })),
)
const Authorization = lazy(() =>
  import('../components/CustomerBatchAuthorizationModal').then((module) => ({
    default: module.CustomerBatchAuthorizationModal,
  })),
)
export function AdminModalHost({
  consoleProps,
  authorization,
}: {
  consoleProps: ComponentProps<typeof ConsoleModals> | null
  authorization: ComponentProps<typeof Authorization> | null
}) {
  return (
    <Suspense fallback={null}>
      {consoleProps ? <ConsoleModals {...consoleProps} /> : null}
      {authorization ? <Authorization {...authorization} /> : null}
    </Suspense>
  )
}
