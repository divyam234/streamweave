import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Boxes,
  Cable,
  CircleCheck,
  Database,
  Film,
  Layers3,
  Plus,
  RadioTower,
  ServerCog,
} from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'

import { api, getAdminSession, loginAdmin, logoutAdmin } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

async function loadStatus() {
  const { data, error } = await api.GET('/api/v1/status')
  if (error || !data) throw new Error('Unable to load service status')
  return data
}

async function loadProviders() {
  const { data, error } = await api.GET('/api/v1/providers')
  if (error || !data) throw new Error('Unable to load providers')
  return data.items
}

async function loadInstallations() {
  const { data, error } = await api.GET('/api/v1/installations')
  if (error || !data) throw new Error('Unable to load installations')
  return data.items
}

async function loadResolvers() {
  const { data, error } = await api.GET('/api/v1/resolvers')
  if (error || !data) throw new Error('Unable to load resolvers')
  return data.items
}

const providerKinds = [
  ['remote-addon', 'Remote Stremio addon'],
  ['torznab', 'Torznab'],
  ['newznab', 'Newznab'],
  ['prowlarr', 'Prowlarr'],
  ['jackett', 'Jackett'],
  ['nzbhydra2', 'NZBHydra2'],
  ['eztv', 'EZTV'],
  ['knaben', 'Knaben'],
  ['the-pirate-bay', 'The Pirate Bay'],
  ['therarbg', 'TheRARBG'],
  ['torrent-galaxy', 'TorrentGalaxy'],
  ['torbox-search', 'TorBox Search'],
] as const

type ProviderKind = (typeof providerKinds)[number][0]

const remoteAddonPresets = [
  ['none', 'Custom addon'],
  ['torrentio', 'Torrentio'],
  ['comet', 'Comet'],
  ['mediafusion', 'MediaFusion'],
] as const

type RemoteAddonPreset = Exclude<(typeof remoteAddonPresets)[number][0], 'none'>

const resolverKinds = [
  ['realdebrid', 'Real-Debrid'],
  ['debridlink', 'Debrid-Link'],
  ['premiumize', 'Premiumize'],
  ['alldebrid', 'AllDebrid'],
  ['torbox', 'TorBox'],
  ['easydebrid', 'EasyDebrid'],
  ['debrider', 'Debrider'],
  ['pikpak', 'PikPak'],
  ['offcloud', 'Offcloud'],
  ['torrin', 'Torrin'],
] as const

type ResolverKind = (typeof resolverKinds)[number][0]

type ControlTab = 'providers' | 'resolvers' | 'installations' | 'architecture'

const controlTabLabels: Record<ControlTab, string> = {
  providers: 'Providers',
  resolvers: 'Resolvers',
  installations: 'Installations',
  architecture: 'Architecture',
}

function Sidebar({
  activeTab,
  onSelect,
}: {
  activeTab: ControlTab
  onSelect: (tab: ControlTab) => void
}) {
  const items = [
    [RadioTower, 'Providers', 'providers'],
    [Cable, 'Resolvers', 'resolvers'],
    [Layers3, 'Installations', 'installations'],
    [ServerCog, 'Architecture', 'architecture'],
  ] as const

  return (
    <aside className="hidden w-64 shrink-0 border-r border-border/60 bg-card/30 lg:flex lg:flex-col">
      <div className="flex h-16 items-center gap-3 border-b border-border/60 px-5">
        <div className="grid size-9 place-items-center rounded-xl bg-primary text-primary-foreground">
          <Film className="size-4" />
        </div>
        <div>
          <div className="text-sm font-semibold">StreamWeave</div>
          <div className="text-xs text-muted-foreground">Control plane</div>
        </div>
      </div>
      <nav className="flex-1 space-y-1 p-3">
        {items.map(([Icon, label, value]) => (
          <button
            aria-current={activeTab === value ? 'page' : undefined}
            className={[
              'flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm transition',
              activeTab === value
                ? 'bg-accent text-accent-foreground'
                : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
            ].join(' ')}
            key={value}
            onClick={() => onSelect(value)}
            type="button"
          >
            <Icon className="size-4" />
            {label}
          </button>
        ))}
      </nav>
      <div className="p-4">
        <div className="rounded-xl border border-border/60 bg-background/50 p-3">
          <div className="flex items-center gap-2 text-xs font-medium">
            <CircleCheck className="size-3.5 text-emerald-400" />
            Contract-first
          </div>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            TypeSpec → OpenAPI → ogen + browser types.
          </p>
        </div>
      </div>
    </aside>
  )
}

function StatCard({
  label,
  value,
  detail,
  icon: Icon,
}: {
  label: string
  value: string
  detail: string
  icon: typeof Database
}) {
  return (
    <Card className="border-border/60 bg-card/60">
      <CardContent className="flex items-start justify-between p-5">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
            {label}
          </p>
          <p className="mt-2 text-2xl font-semibold tracking-tight">{value}</p>
          <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
        </div>
        <div className="rounded-lg border border-border/60 bg-background/70 p-2.5">
          <Icon className="size-4 text-muted-foreground" />
        </div>
      </CardContent>
    </Card>
  )
}

function App() {
  const queryClient = useQueryClient()
  const [adminTokenInput, setAdminTokenInput] = useState('')
  const [authError, setAuthError] = useState('')
  const [authBusy, setAuthBusy] = useState(false)
  const [uiMessage, setUiMessage] = useState('')

  const [providerKind, setProviderKind] = useState<ProviderKind>('remote-addon')
  const [resolverKind, setResolverKind] = useState<ResolverKind>('realdebrid')
  const [providerPreset, setProviderPreset] = useState<(typeof remoteAddonPresets)[number][0]>('none')
  const [installationClientMode, setInstallationClientMode] = useState<'stremio' | 'nuvio' | 'universal'>('universal')
  const [installationResolutionMode, setInstallationResolutionMode] = useState<'client' | 'server' | 'hybrid'>('hybrid')
  const [installationResolverId, setInstallationResolverId] = useState('none')
  const [activeTab, setActiveTab] = useState<ControlTab>('providers')

  const flash = (message: string) => {
    setUiMessage(message)
    window.setTimeout(() => setUiMessage(''), 2500)
  }
  const adminSession = useQuery({
    queryKey: ['admin-session'],
    queryFn: getAdminSession,
    retry: false,
    refetchInterval: 5 * 60_000,
    refetchOnWindowFocus: true,
  })

  useEffect(() => {
    const handleExpired = () => {
      queryClient.setQueryData(['admin-session'], false)
      queryClient.removeQueries({ queryKey: ['status'] })
      queryClient.removeQueries({ queryKey: ['providers'] })
      queryClient.removeQueries({ queryKey: ['resolvers'] })
      queryClient.removeQueries({ queryKey: ['installations'] })
      setAuthError('Admin session expired. Unlock again to continue.')
    }
    window.addEventListener('streamweave-auth-expired', handleExpired)
    return () => window.removeEventListener('streamweave-auth-expired', handleExpired)
  }, [queryClient])
  const hasAdminToken = adminSession.data === true
  const status = useQuery({
    queryKey: ['status'],
    queryFn: loadStatus,
    refetchInterval: 15_000,
    enabled: hasAdminToken,
  })
  const providers = useQuery({ queryKey: ['providers'], queryFn: loadProviders, enabled: hasAdminToken })
  const installations = useQuery({
    queryKey: ['installations'],
    queryFn: loadInstallations,
    enabled: hasAdminToken,
  })
  const resolvers = useQuery({
    queryKey: ['resolvers'],
    queryFn: loadResolvers,
    enabled: hasAdminToken,
  })

  const createProvider = useMutation({
    mutationFn: async (input: {
      name: string
      kind: ProviderKind
      endpoint: string
      apiKey?: string
      preset?: RemoteAddonPreset
    }) => {
      const { data, error } = await api.POST('/api/v1/providers', {
        body: {
          name: input.name,
          kind: input.kind,
          endpoint: input.endpoint,
          apiKey: input.apiKey || undefined,
          preset: input.preset,
          enabled: true,
        },
      })
      if (error || !data) throw new Error('Unable to create provider')
      return data
    },
    onSuccess: () => {
      flash('Provider added')
      void queryClient.invalidateQueries({ queryKey: ['providers'] })
    },
  })

  const setProviderEnabled = useMutation({
    mutationFn: async (input: { id: string; enabled: boolean }) => {
      const { data, error } = await api.PATCH('/api/v1/providers/{id}', {
        params: { path: { id: input.id } },
        body: { enabled: input.enabled },
      })
      if (error || !data) throw new Error('Unable to update provider')
      return data
    },
    onSuccess: (data) => {
      flash(`Provider ${data.enabled ? 'enabled' : 'disabled'}`)
      void queryClient.invalidateQueries({ queryKey: ['providers'] })
    },
  })

  const createResolver = useMutation({
    mutationFn: async (input: { name: string; kind: ResolverKind; apiKey: string; proxyUrl?: string }) => {
      const { data, error } = await api.POST('/api/v1/resolvers', {
        body: {
          name: input.name,
          kind: input.kind,
          apiKey: input.apiKey,
          proxyUrl: input.proxyUrl,
          enabled: true,
        },
      })
      if (error || !data) throw new Error('Unable to create resolver')
      return data
    },
    onSuccess: () => {
      flash('Resolver added')
      void queryClient.invalidateQueries({ queryKey: ['resolvers'] })
    },
  })

  const setResolverEnabled = useMutation({
    mutationFn: async (input: { id: string; enabled: boolean }) => {
      const { data, error } = await api.PATCH('/api/v1/resolvers/{id}', {
        params: { path: { id: input.id } },
        body: { enabled: input.enabled },
      })
      if (error || !data) throw new Error('Unable to update resolver')
      return data
    },
    onSuccess: (data) => {
      flash(`Resolver ${data.enabled ? 'enabled' : 'disabled'}`)
      void queryClient.invalidateQueries({ queryKey: ['resolvers'] })
    },
  })

  const createInstallation = useMutation({
    mutationFn: async (input: {
      name: string
      clientMode: 'stremio' | 'nuvio' | 'universal'
      resolutionMode: 'client' | 'server' | 'hybrid'
      resolverId?: string
    }) => {
      const { data, error } = await api.POST('/api/v1/installations', {
        body: {
          name: input.name,
          clientMode: input.clientMode,
          resolutionMode: input.resolutionMode,
          resolverId: input.resolverId || undefined,
          enabled: true,
        },
      })
      if (error || !data) throw new Error('Unable to create installation')
      return data
    },
    onSuccess: () => {
      flash('Installation created')
      void queryClient.invalidateQueries({ queryKey: ['installations'] })
    },
  })

  const setInstallationEnabled = useMutation({
    mutationFn: async (input: { id: string; enabled: boolean }) => {
      const { data, error } = await api.PATCH('/api/v1/installations/{id}', {
        params: { path: { id: input.id } },
        body: { enabled: input.enabled },
      })
      if (error || !data) throw new Error('Unable to update installation')
      return data
    },
    onSuccess: (data) => {
      flash(`Installation ${data.enabled ? 'enabled' : 'disabled'}`)
      void queryClient.invalidateQueries({ queryKey: ['installations'] })
    },
  })

  const rotateInstallationToken = useMutation({
    mutationFn: async (id: string) => {
      const { data, error } = await api.POST('/api/v1/installations/{id}/rotate-token', {
        params: { path: { id } },
      })
      if (error || !data) throw new Error('Unable to rotate installation token')
      return data
    },
    onSuccess: () => {
      flash('Installation token rotated; the previous addon URL is now invalid')
      void queryClient.invalidateQueries({ queryKey: ['installations'] })
    },
  })

  const copyInstallationURL = async (id: string) => {
    const url = `${window.location.origin}/addon/${id}/manifest.json`
    try {
      await navigator.clipboard.writeText(url)
      flash('Installation URL copied')
    } catch {
      window.prompt('Copy this installation URL:', url)
    }
  }

  const submitProvider = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = event.currentTarget
    const data = new FormData(form)
    createProvider.mutate(
      {
        name: String(data.get('name') ?? '').trim(),
        kind: providerKind,
        preset:
          providerKind === 'remote-addon' && providerPreset !== 'none'
            ? (providerPreset as RemoteAddonPreset)
            : undefined,
        endpoint: String(data.get('endpoint') ?? '').trim(),
        apiKey: String(data.get('apiKey') ?? '').trim() || undefined,
      },
      {
        onSuccess: () => {
          form.reset()
          setProviderKind('remote-addon')
          setProviderPreset('none')
        },
      },
    )
  }

  const submitResolver = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = event.currentTarget
    const data = new FormData(form)
    createResolver.mutate(
      {
        name: String(data.get('name') ?? '').trim(),
        kind: resolverKind,
        apiKey: String(data.get('apiKey') ?? '').trim(),
        proxyUrl: String(data.get('proxyUrl') ?? '').trim() || undefined,
      },
      {
        onSuccess: () => {
          form.reset()
          setResolverKind('realdebrid')
        },
      },
    )
  }

  const submitInstallation = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = event.currentTarget
    const data = new FormData(form)
    const resolverId =
      installationResolutionMode === 'client' || installationResolverId === 'none'
        ? ''
        : installationResolverId
    createInstallation.mutate(
      {
        name: String(data.get('name') ?? '').trim(),
        clientMode: installationClientMode,
        resolutionMode: installationResolutionMode,
        resolverId: resolverId || undefined,
      },
      {
        onSuccess: () => {
          form.reset()
          setInstallationClientMode('universal')
          setInstallationResolutionMode('hybrid')
          setInstallationResolverId('none')
        },
      },
    )
  }

  const submitAdminToken = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const token = adminTokenInput.trim()
    if (!token || authBusy) return
    setAuthError('')
    setAuthBusy(true)
    void loginAdmin(token)
      .then(async () => {
        setAdminTokenInput('')
        await queryClient.invalidateQueries({ queryKey: ['admin-session'] })
        await queryClient.invalidateQueries()
      })
      .catch((error) => {
        setAuthError(error instanceof Error ? error.message : 'Unable to unlock admin')
      })
      .finally(() => setAuthBusy(false))
  }

  const clearAdminToken = () => {
    if (authBusy) return
    setAuthError('')
    setAuthBusy(true)
    void logoutAdmin()
      .then(() => {
        queryClient.setQueryData(['admin-session'], false)
        queryClient.removeQueries({ queryKey: ['status'] })
        queryClient.removeQueries({ queryKey: ['providers'] })
        queryClient.removeQueries({ queryKey: ['resolvers'] })
        queryClient.removeQueries({ queryKey: ['installations'] })
      })
      .catch((error) => {
        setAuthError(error instanceof Error ? error.message : 'Unable to lock admin')
      })
      .finally(() => setAuthBusy(false))
  }

  const databaseUp = status.data?.database === 'up'

  const operationError = [
    setProviderEnabled.error,
    setResolverEnabled.error,
    setInstallationEnabled.error,
    rotateInstallationToken.error,
  ].find((error): error is Error => error instanceof Error)

  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto flex min-h-screen max-w-[1680px] border-x border-border/40">
        {hasAdminToken && <Sidebar activeTab={activeTab} onSelect={setActiveTab} />}

        <main className="min-w-0 flex-1">
          <header className="flex min-h-16 flex-wrap items-center justify-between gap-3 border-b border-border/60 bg-background/80 px-5 py-3 backdrop-blur md:px-7">
            <div>
              <h1 className="text-sm font-semibold">
                {adminSession.isError
                  ? 'Backend unavailable'
                  : hasAdminToken
                    ? controlTabLabels[activeTab]
                    : 'Admin access'}
              </h1>
              <p className="text-xs text-muted-foreground">
                {adminSession.isError
                  ? 'Cannot reach the control-plane session endpoint'
                  : hasAdminToken
                    ? 'Aggregation runtime and client configuration'
                    : 'Unlock the secure control plane to continue'}
              </p>
            </div>
            <div className="flex items-center gap-2">
              {hasAdminToken ? (
                <Button disabled={authBusy} size="sm" variant="outline" onClick={clearAdminToken}>
                  Lock admin
                </Button>
              ) : (
                <form className="flex items-center gap-2" onSubmit={submitAdminToken}>
                  <Input
                    aria-label="Admin token"
                    autoComplete="off"
                    className="h-8 w-40 sm:w-56"
                    onChange={(event) => setAdminTokenInput(event.target.value)}
                    placeholder="Admin token"
                    type="password"
                    value={adminTokenInput}
                  />
                  <Button disabled={authBusy || !adminTokenInput.trim()} size="sm" type="submit">
                    {authBusy ? 'Unlocking…' : 'Unlock'}
                  </Button>
                </form>
              )}
              <Badge variant="outline" className="gap-1.5">
                <span
                  className={[
                    'size-1.5 rounded-full',
                    hasAdminToken && status.data?.status === 'ok'
                      ? 'bg-emerald-400'
                      : adminSession.isPending
                        ? 'bg-muted-foreground'
                        : 'bg-amber-400',
                  ].join(' ')}
                />
                {adminSession.isPending
                  ? 'Checking session'
                  : adminSession.isError
                    ? 'Backend unavailable'
                    : !hasAdminToken
                      ? 'Admin locked'
                      : status.isPending
                        ? 'Connecting'
                        : status.isError
                          ? 'API unavailable'
                          : 'API online'}
              </Badge>
              <Badge className="hidden sm:inline-flex" variant="secondary">v{status.data?.version ?? '—'}</Badge>
            </div>
          </header>

          <div className="space-y-6 p-5 md:p-7">

            {authError && (
              <div role="alert" className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {authError}
              </div>
            )}

            {hasAdminToken && operationError && (
              <div role="alert" className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {operationError.message}
              </div>
            )}
            {hasAdminToken && status.isError && (
              <div role="alert" className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                Control API is unavailable. Check the application health and reverse-proxy connection.
              </div>
            )}
            {hasAdminToken && status.data && !databaseUp && (
              <div role="alert" className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-300">
                PostgreSQL is unavailable. Configuration changes are disabled until the database recovers.
              </div>
            )}
            {hasAdminToken && uiMessage && (
              <div role="status" aria-live="polite" className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-300">
                {uiMessage}
              </div>
            )}
            {adminSession.isPending ? (
              <Card className="border-border/60">
                <CardHeader>
                  <CardTitle>Checking admin session…</CardTitle>
                  <CardDescription>Restoring your secure HttpOnly session.</CardDescription>
                </CardHeader>
              </Card>
            ) : adminSession.isError ? (
              <Card className="border-destructive/30">
                <CardHeader>
                  <CardTitle>Control plane unavailable</CardTitle>
                  <CardDescription>
                    The browser cannot reach the authentication/session endpoint. Check the backend and reverse proxy, then retry.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <Button variant="outline" onClick={() => void adminSession.refetch()}>
                    Retry connection
                  </Button>
                </CardContent>
              </Card>
            ) : !hasAdminToken ? (
              <Card className="border-border/60">
                <CardHeader>
                  <CardTitle>Admin access required</CardTitle>
                  <CardDescription>
                    Unlock the control plane with your admin token. The token is exchanged for an HttpOnly session cookie and is never saved in browser storage.
                  </CardDescription>
                </CardHeader>
              </Card>
            ) : (
              <>
            <section>
              <div className="mb-4">
                <h2 className="text-xl font-semibold tracking-tight">Runtime</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  Shared engine for Stremio and Nuvio protocol clients.
                </p>
              </div>
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                <StatCard
                  icon={Database}
                  label="Database"
                  value={status.data?.database ?? 'unknown'}
                  detail="PostgreSQL · pgx · sqlc"
                />
                <StatCard
                  icon={RadioTower}
                  label="Providers"
                  value={String(providers.data?.length ?? 0)}
                  detail="Bounded concurrent discovery"
                />
                <StatCard
                  icon={Boxes}
                  label="Installations"
                  value={String(installations.data?.length ?? 0)}
                  detail="Stremio · Nuvio · universal"
                />
                <StatCard
                  icon={Cable}
                  label="Resolvers"
                  value={String(resolvers.data?.length ?? 0)}
                  detail={status.data?.secrets === 'ready' ? 'Encrypted credentials ready' : 'MASTER_KEY required'}
                />
              </div>
            </section>

            <Tabs
              value={activeTab}
              onValueChange={(value) => setActiveTab(value as ControlTab)}
              className="space-y-4"
            >
              <TabsList className="max-w-full justify-start overflow-x-auto">
                <TabsTrigger value="providers">Providers</TabsTrigger>
                <TabsTrigger value="resolvers">Resolvers</TabsTrigger>
                <TabsTrigger value="installations">Installations</TabsTrigger>
                <TabsTrigger value="architecture">Architecture</TabsTrigger>
              </TabsList>

              <TabsContent value="providers" className="grid gap-4 xl:grid-cols-[1fr_360px]">
                <Card className="border-border/60">
                  <CardHeader>
                    <CardTitle>Discovery providers</CardTitle>
                    <CardDescription>
                      Enabled remote addons are loaded from PostgreSQL for each discovery plan.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    {providers.isPending ? (
                      <div className="space-y-2">
                        <Skeleton className="h-10 w-full" />
                        <Skeleton className="h-10 w-full" />
                      </div>
                    ) : providers.isError ? (
                      <p role="alert" className="text-sm text-destructive">
                        Unable to load providers. Check the admin session and database connectivity.
                      </p>
                    ) : (
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>Name</TableHead>
                            <TableHead>Kind</TableHead>
                            <TableHead>Endpoint</TableHead>
                            <TableHead className="text-right">Actions</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {(providers.data ?? []).map((provider) => (
                            <TableRow key={provider.id}>
                              <TableCell className="font-medium">{provider.name}</TableCell>
                              <TableCell>
                                <Badge variant="secondary">{provider.kind}</Badge>
                              </TableCell>
                              <TableCell className="max-w-[340px] truncate font-mono text-xs text-muted-foreground">
                                {provider.endpoint || 'Built-in default'}
                              </TableCell>
                              <TableCell className="text-right">
                                <div className="flex justify-end gap-2">
                                  <Badge variant={provider.enabled ? 'default' : 'outline'}>
                                    {provider.enabled ? 'Enabled' : 'Disabled'}
                                  </Badge>
                                  <Button
                                    disabled={setProviderEnabled.isPending}
                                    size="sm"
                                    variant="outline"
                                    onClick={() =>
                                      setProviderEnabled.mutate({
                                        id: provider.id,
                                        enabled: !provider.enabled,
                                      })
                                    }
                                  >
                                    {provider.enabled ? 'Disable' : 'Enable'}
                                  </Button>
                                </div>
                              </TableCell>
                            </TableRow>
                          ))}
                          {!providers.data?.length && (
                            <TableRow>
                              <TableCell colSpan={4} className="h-24 text-center text-muted-foreground">
                                No providers configured.
                              </TableCell>
                            </TableRow>
                          )}
                        </TableBody>
                      </Table>
                    )}
                  </CardContent>
                </Card>

                <Card className="border-border/60">
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <Plus className="size-4" /> Add provider
                    </CardTitle>
                    <CardDescription>
                      Add a native indexer/search provider or a Stremio-compatible remote addon.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <form className="space-y-4" onSubmit={submitProvider}>
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium" htmlFor="provider-name">
                          Name
                        </label>
                        <Input id="provider-name" name="name" placeholder="My provider" required />
                      </div>
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium">Kind</label>
                        <Select
                          name="kind"
                          value={providerKind}
                          onValueChange={(value) => {
                            const next = value as ProviderKind
                            setProviderKind(next)
                            if (next !== 'remote-addon') setProviderPreset('none')
                          }}
                        >
                          <SelectTrigger aria-label="Provider kind">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {providerKinds.map(([value, label]) => (
                              <SelectItem key={value} value={value}>
                                {label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                      {providerKind === 'remote-addon' && (
                        <div className="space-y-1.5">
                          <label className="text-xs font-medium">Remote addon preset</label>
                          <Select
                            name="preset"
                            value={providerPreset}
                            onValueChange={(value) =>
                              setProviderPreset(value as (typeof remoteAddonPresets)[number][0])
                            }
                          >
                            <SelectTrigger aria-label="Remote addon preset">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {remoteAddonPresets.map(([value, label]) => (
                                <SelectItem key={value} value={value}>
                                  {label}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                          <p className="text-xs text-muted-foreground">
                            Torrentio, Comet, and MediaFusion use maintained public defaults. Paste a configured manifest URL below to override the preset.
                          </p>
                        </div>
                      )}
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium" htmlFor="provider-endpoint">
                          Endpoint
                        </label>
                        <Input
                          id="provider-endpoint"
                          name="endpoint"
                          placeholder="Leave blank for built-in defaults"
                          type="url"
                          required={
                            ['torznab', 'newznab', 'prowlarr', 'jackett', 'nzbhydra2'].includes(
                              providerKind,
                            ) ||
                            (providerKind === 'remote-addon' && providerPreset === 'none')
                          }
                        />
                        <p className="text-xs text-muted-foreground">
                          Remote addons and self-hosted indexers need an endpoint. Built-in public sources can use their defaults.
                        </p>
                      </div>
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium" htmlFor="provider-key">
                          API key <span className="text-muted-foreground">(when required)</span>
                        </label>
                        <Input
                          autoComplete="new-password"
                          id="provider-key"
                          name="apiKey"
                          placeholder="Prowlarr, Torznab, Newznab, TorBox Search…"
                          type="password"
                          required={['prowlarr', 'torbox-search'].includes(providerKind)}
                        />
                      </div>
                      <Button
                        className="w-full"
                        disabled={!databaseUp || createProvider.isPending}
                        type="submit"
                      >
                        <Plus className="size-4" />
                        {createProvider.isPending ? 'Adding…' : 'Add provider'}
                      </Button>
                      {createProvider.isError && (
                        <p className="text-xs text-destructive">
                          {createProvider.error instanceof Error
                            ? createProvider.error.message
                            : 'Provider creation failed'}
                        </p>
                      )}
                    </form>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="resolvers" className="grid gap-4 xl:grid-cols-[1fr_360px]">
                <Card className="border-border/60">
                  <CardHeader>
                    <CardTitle>Debrid resolvers</CardTitle>
                    <CardDescription>
                      Credentials are encrypted with AES-256-GCM before they are written to PostgreSQL.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    {resolvers.isPending ? (
                      <div className="space-y-2">
                        <Skeleton className="h-10 w-full" />
                        <Skeleton className="h-10 w-full" />
                      </div>
                    ) : resolvers.isError ? (
                      <p role="alert" className="text-sm text-destructive">
                        Unable to load resolver accounts. Check the admin session and database connectivity.
                      </p>
                    ) : (
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>Name</TableHead>
                            <TableHead>Service</TableHead>
                            <TableHead className="text-right">Actions</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {(resolvers.data ?? []).map((resolver) => (
                            <TableRow key={resolver.id}>
                              <TableCell className="font-medium">{resolver.name}</TableCell>
                              <TableCell>
                                <Badge variant="secondary">{resolver.kind}</Badge>
                              </TableCell>
                              <TableCell className="text-right">
                                <div className="flex justify-end gap-2">
                                  <Badge variant={resolver.enabled ? 'default' : 'outline'}>
                                    {resolver.enabled ? 'Enabled' : 'Disabled'}
                                  </Badge>
                                  {resolver.proxyEnabled && <Badge variant="secondary">Proxied</Badge>}
                                  <Button
                                    disabled={setResolverEnabled.isPending}
                                    size="sm"
                                    variant="outline"
                                    onClick={() =>
                                      setResolverEnabled.mutate({
                                        id: resolver.id,
                                        enabled: !resolver.enabled,
                                      })
                                    }
                                  >
                                    {resolver.enabled ? 'Disable' : 'Enable'}
                                  </Button>
                                </div>
                              </TableCell>
                            </TableRow>
                          ))}
                          {!resolvers.data?.length && (
                            <TableRow>
                              <TableCell colSpan={3} className="h-24 text-center text-muted-foreground">
                                No resolver accounts configured.
                              </TableCell>
                            </TableRow>
                          )}
                        </TableBody>
                      </Table>
                    )}
                  </CardContent>
                </Card>

                <Card className="border-border/60">
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <Plus className="size-4" /> Add resolver
                    </CardTitle>
                    <CardDescription>
                      Resolver credentials are accepted once, encrypted, and never returned by the API.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <form className="space-y-4" onSubmit={submitResolver}>
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium" htmlFor="resolver-name">
                          Name
                        </label>
                        <Input id="resolver-name" name="name" placeholder="My debrid account" required />
                      </div>
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium">Service</label>
                        <Select
                          name="kind"
                          value={resolverKind}
                          onValueChange={(value) => setResolverKind(value as ResolverKind)}
                        >
                          <SelectTrigger aria-label="Resolver service">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {resolverKinds.map(([value, label]) => (
                              <SelectItem key={value} value={value}>
                                {label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium" htmlFor="resolver-key">
                          {resolverKind === 'pikpak' ? 'Username and password' : 'API key / token'}
                        </label>
                        <Input
                          autoComplete="new-password"
                          id="resolver-key"
                          name="apiKey"
                          placeholder={
                            resolverKind === 'pikpak'
                              ? 'username@example.com:password'
                              : 'Service API key or token'
                          }
                          required
                          type="password"
                        />
                        <p className="text-xs text-muted-foreground">
                          {resolverKind === 'pikpak'
                            ? 'Enter the PikPak login in username:password format. It is encrypted before storage.'
                            : 'The credential is accepted once, encrypted at rest, and never returned by the API.'}
                        </p>
                      </div>
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium" htmlFor="resolver-proxy">Outbound proxy URL (optional)</label>
                        <Input id="resolver-proxy" name="proxyUrl" placeholder="socks5://user:password@proxy.example:1080" autoComplete="off" type="password" />
                        <p className="text-xs text-muted-foreground">Debrid API calls and video playback will use this proxy. The URL is encrypted and never returned.</p>
                      </div>
                      <Button
                        className="w-full"
                        disabled={
                          !databaseUp ||
                          status.data?.secrets !== 'ready' ||
                          createResolver.isPending
                        }
                        type="submit"
                      >
                        <Plus className="size-4" />
                        {createResolver.isPending ? 'Encrypting…' : 'Add resolver'}
                      </Button>
                      {status.data?.secrets !== 'ready' && (
                        <p className="text-xs text-amber-400">
                          Set a 64-character hex MASTER_KEY before storing resolver credentials.
                        </p>
                      )}
                      {createResolver.isError && (
                        <p className="text-xs text-destructive">
                          {createResolver.error instanceof Error
                            ? createResolver.error.message
                            : 'Resolver creation failed'}
                        </p>
                      )}
                    </form>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent
                value="installations"
                className="grid gap-4 xl:grid-cols-[1fr_360px]"
              >
                <Card className="border-border/60">
                  <CardHeader>
                    <CardTitle>Client installations</CardTitle>
                    <CardDescription>
                      Profiles become the home for ranking, resolver, filter, and language policy.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    {installations.isPending ? (
                      <div className="space-y-2">
                        <Skeleton className="h-10 w-full" />
                        <Skeleton className="h-10 w-full" />
                      </div>
                    ) : installations.isError ? (
                      <p role="alert" className="text-sm text-destructive">
                        Unable to load installations. Check the admin session and database connectivity.
                      </p>
                    ) : (
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>Name</TableHead>
                            <TableHead>Client mode</TableHead>
                            <TableHead>Resolution</TableHead>
                            <TableHead>Resolver</TableHead>
                            <TableHead>Addon URL</TableHead>
                            <TableHead className="text-right">Actions</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {(installations.data ?? []).map((installation) => (
                            <TableRow key={installation.id}>
                              <TableCell className="font-medium">
                                <div className="flex items-center gap-2">
                                  <span>{installation.name}</span>
                                  {!installation.enabled && <Badge variant="outline">Disabled</Badge>}
                                </div>
                              </TableCell>
                              <TableCell>
                                <Badge variant="secondary">{installation.clientMode}</Badge>
                              </TableCell>
                              <TableCell>
                                <Badge variant="outline">{installation.resolutionMode}</Badge>
                              </TableCell>
                              <TableCell className="text-muted-foreground">
                                {resolvers.data?.find((resolver) => resolver.id === installation.resolverId)?.name ?? '—'}
                              </TableCell>
                              <TableCell>
                                <a
                                  className="font-mono text-xs text-primary hover:underline"
                                  href={`/addon/${installation.id}/manifest.json`}
                                  rel="noreferrer"
                                  target="_blank"
                                >
                                  /addon/{installation.id.slice(0, 8)}…/manifest.json
                                </a>
                              </TableCell>
                              <TableCell>
                                <div className="flex flex-wrap justify-end gap-2">
                                  <Button
                                    size="sm"
                                    variant="outline"
                                    onClick={() => void copyInstallationURL(installation.id)}
                                  >
                                    Copy URL
                                  </Button>
                                  <Button
                                    disabled={setInstallationEnabled.isPending}
                                    size="sm"
                                    variant="outline"
                                    onClick={() =>
                                      setInstallationEnabled.mutate({
                                        id: installation.id,
                                        enabled: !installation.enabled,
                                      })
                                    }
                                  >
                                    {installation.enabled ? 'Disable' : 'Enable'}
                                  </Button>
                                  <Button
                                    disabled={rotateInstallationToken.isPending}
                                    size="sm"
                                    variant="outline"
                                    onClick={() => {
                                      if (
                                        window.confirm(
                                          'Rotate this installation token? The current addon URL will stop working immediately.',
                                        )
                                      ) {
                                        rotateInstallationToken.mutate(installation.id)
                                      }
                                    }}
                                  >
                                    {rotateInstallationToken.isPending ? 'Rotating…' : 'Rotate'}
                                  </Button>
                                </div>
                              </TableCell>
                            </TableRow>
                          ))}
                          {!installations.data?.length && (
                            <TableRow>
                              <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
                                No installations configured.
                              </TableCell>
                            </TableRow>
                          )}
                        </TableBody>
                      </Table>
                    )}
                  </CardContent>
                </Card>

                <Card className="border-border/60">
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <Plus className="size-4" /> Create installation
                    </CardTitle>
                    <CardDescription>Create a client policy profile.</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <form className="space-y-4" onSubmit={submitInstallation}>
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium" htmlFor="installation-name">
                          Name
                        </label>
                        <Input
                          id="installation-name"
                          name="name"
                          placeholder="Living room"
                          required
                        />
                      </div>
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium">Client mode</label>
                        <Select
                          name="clientMode"
                          value={installationClientMode}
                          onValueChange={(value) =>
                            setInstallationClientMode(value as 'stremio' | 'nuvio' | 'universal')
                          }
                        >
                          <SelectTrigger aria-label="Client mode" className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="universal">Universal</SelectItem>
                            <SelectItem value="stremio">Stremio</SelectItem>
                            <SelectItem value="nuvio">Nuvio</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium">Resolution mode</label>
                        <Select
                          name="resolutionMode"
                          value={installationResolutionMode}
                          onValueChange={(value) => {
                            const next = value as 'client' | 'server' | 'hybrid'
                            setInstallationResolutionMode(next)
                            if (next === 'client') setInstallationResolverId('none')
                          }}
                        >
                          <SelectTrigger aria-label="Resolution mode" className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="hybrid">Hybrid — resolve cached, keep fallback</SelectItem>
                            <SelectItem value="server">Server — resolved links only</SelectItem>
                            <SelectItem value="client">Client — raw torrent/direct sources</SelectItem>
                          </SelectContent>
                        </Select>
                        <p className="text-xs text-muted-foreground">
                          Client mode returns raw torrent/direct sources. Server mode requires a resolver. Hybrid resolves server-side when possible and preserves fallback.
                        </p>
                        {installationClientMode === 'nuvio' && installationResolutionMode === 'client' && (
                          <p className="text-xs text-emerald-400">
                            Nuvio native debrid mode: raw torrent hashes will be returned for Nuvio to resolve.
                          </p>
                        )}
                      </div>
                      {installationResolutionMode !== 'client' && (
                        <div className="space-y-1.5">
                          <label className="text-xs font-medium">Resolver</label>
                          <Select
                            name="resolverId"
                            value={installationResolverId}
                            onValueChange={setInstallationResolverId}
                          >
                            <SelectTrigger aria-label="Resolver" className="w-full">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="none">
                                {installationResolutionMode === 'server'
                                  ? 'Select a resolver'
                                  : 'No resolver — client fallback only'}
                              </SelectItem>
                              {(resolvers.data ?? [])
                                .filter((resolver) => resolver.enabled)
                                .map((resolver) => (
                                  <SelectItem key={resolver.id} value={resolver.id}>
                                    {resolver.name}
                                  </SelectItem>
                                ))}
                            </SelectContent>
                          </Select>
                          {installationResolutionMode === 'server' && installationResolverId === 'none' && (
                            <p className="text-xs text-amber-400">
                              Server mode requires an enabled resolver.
                            </p>
                          )}
                        </div>
                      )}
                      <Button
                        className="w-full"
                        disabled={
                          !databaseUp ||
                          createInstallation.isPending ||
                          (installationResolutionMode === 'server' &&
                            installationResolverId === 'none')
                        }
                        type="submit"
                      >
                        <Plus className="size-4" />
                        {createInstallation.isPending ? 'Creating…' : 'Create installation'}
                      </Button>
                      {createInstallation.isError && (
                        <p role="alert" className="text-xs text-destructive">
                          {createInstallation.error instanceof Error
                            ? createInstallation.error.message
                            : 'Installation creation failed'}
                        </p>
                      )}
                    </form>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="architecture">
                <Card className="border-border/60">
                  <CardHeader>
                    <CardTitle>Architecture baseline</CardTitle>
                    <CardDescription>
                      Generated boundaries outside, strongly typed deterministic engine inside.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                      {[
                        [ServerCog, 'Control API', 'TypeSpec → OpenAPI 3.1 → ogen'],
                        [Database, 'Persistence', 'PostgreSQL → sqlc → pgx'],
                        [RadioTower, 'Discovery', 'Bounded errgroup provider fan-out'],
                        [Cable, 'Protocol', 'Stremio core + Nuvio adaptation'],
                      ].map(([Icon, title, text]) => {
                        const ArchitectureIcon = Icon as typeof ServerCog
                        return (
                          <div
                            className="rounded-xl border border-border/60 bg-muted/20 p-4"
                            key={String(title)}
                          >
                            <ArchitectureIcon className="size-4 text-muted-foreground" />
                            <p className="mt-3 text-sm font-medium">{String(title)}</p>
                            <p className="mt-1 text-xs leading-5 text-muted-foreground">
                              {String(text)}
                            </p>
                          </div>
                        )
                      })}
                    </div>
                    <Separator className="my-5" />
                    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span>Protocol URL:</span>
                      <code className="rounded-md bg-muted px-2 py-1 text-foreground">
                        /addon/&lt;installation-token&gt;/manifest.json
                      </code>
                    </div>
                  </CardContent>
                </Card>
              </TabsContent>
            </Tabs>
              </>
            )}
          </div>
        </main>
      </div>
    </div>
  )
}

export default App
