import { Link, useLocation } from 'react-router-dom'
import { useUnsavedConfig } from '../contexts/UnsavedConfigContext'

type SafeLinkProps = React.ComponentProps<typeof Link>

function getPath(to: SafeLinkProps['to']): string {
  if (typeof to === 'string') return to
  if (to && typeof to === 'object' && 'pathname' in to) return (to as { pathname: string }).pathname ?? ''
  return ''
}

export function SafeLink({ to, onClick, ...rest }: SafeLinkProps) {
  const loc = useLocation()
  const unsaved = useUnsavedConfig()
  const target = getPath(to)
  const isConfigPage = loc.pathname.startsWith('/admin/config')
  const isLeavingConfig = isConfigPage && target !== '' && !target.startsWith('/admin/config')

  const handleClick = (e: React.MouseEvent<HTMLAnchorElement>) => {
    if (unsaved?.hasUnsaved && isLeavingConfig) {
      unsaved.tryLeave(target, e)
      return
    }
    onClick?.(e as React.MouseEvent<HTMLAnchorElement>)
  }

  return <Link to={to} onClick={handleClick} {...rest} />
}
