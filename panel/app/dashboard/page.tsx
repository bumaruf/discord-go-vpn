import { PageHeader } from '@/components/ui/page-header'
import { monthlyQuotaGb, serverName, wireguardPort } from '@/lib/env'
import { sumMonthUsage, sumServerUsage } from '@/lib/usage'
import { Overview } from './overview'
import { ROUTES } from './routes'

export const dynamic = 'force-dynamic'

export default async function OverviewPage() {
  const [month, history] = await Promise.all([sumMonthUsage(), sumServerUsage()])

  return (
    <>
      <PageHeader
        title={ROUTES.overview.title}
        description={`Tráfego ao vivo e consumo do servidor ${serverName()}, com WireGuard na porta ${wireguardPort()}.`}
      />
      <Overview month={month} history={history} quotaGb={monthlyQuotaGb()} />
    </>
  )
}
