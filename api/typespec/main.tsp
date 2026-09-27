import "@typespec/http";

using Http;

@service(#{ title: "Media Engine Control API" })
@server("http://localhost:8080", "Local development")
namespace MediaEngine;

union ProviderKind {
  remoteAddon: "remote-addon",
  torznab: "torznab",
  newznab: "newznab",
  prowlarr: "prowlarr",
  jackett: "jackett",
  nzbhydra2: "nzbhydra2",
  eztv: "eztv",
  knaben: "knaben",
  thePirateBay: "the-pirate-bay",
  therarbg: "therarbg",
  torrentGalaxy: "torrent-galaxy",
  torboxSearch: "torbox-search",
}

union RemoteAddonPreset {
  torrentio: "torrentio",
  comet: "comet",
  mediafusion: "mediafusion",
}

union ResolverKind {
  realdebrid: "realdebrid",
  debridlink: "debridlink",
  premiumize: "premiumize",
  alldebrid: "alldebrid",
  torbox: "torbox",
  easydebrid: "easydebrid",
  debrider: "debrider",
  pikpak: "pikpak",
  offcloud: "offcloud",
  torrin: "torrin",
}

model StatusResponse {
  status: "ok";
  version: string;
  database: "up" | "down" | "disabled";
  secrets: "ready" | "disabled";
}

model Provider {
  id: string;
  name: string;
  kind: ProviderKind;
  endpoint: string;
  enabled: boolean;
}

model CreateProviderRequest {
  name: string;
  kind: ProviderKind;
  preset?: RemoteAddonPreset;
  endpoint: string;
  apiKey?: string;
  enabled: boolean;
}

model UpdateProviderRequest {
  enabled: boolean;
}

model ProviderListResponse {
  items: Provider[];
}

model ResolverAccount {
  id: string;
  name: string;
  kind: ResolverKind;
  enabled: boolean;
}

model CreateResolverAccountRequest {
  name: string;
  kind: ResolverKind;
  apiKey: string;
  enabled: boolean;
}

model UpdateResolverAccountRequest {
  enabled: boolean;
}

model ResolverAccountListResponse {
  items: ResolverAccount[];
}

model Installation {
  id: string;
  name: string;
  clientMode: "stremio" | "nuvio" | "universal";
  resolutionMode: "client" | "server" | "hybrid";
  resolverId?: string;
  enabled: boolean;
}

model CreateInstallationRequest {
  name: string;
  clientMode: "stremio" | "nuvio" | "universal";
  resolutionMode: "client" | "server" | "hybrid";
  resolverId?: string;
  enabled: boolean;
}

model UpdateInstallationRequest {
  enabled: boolean;
}

model InstallationListResponse {
  items: Installation[];
}

@route("/api/v1")
interface ControlApi {
  @route("/status")
  @get
  getStatus(): StatusResponse;

  @route("/providers")
  @get
  listProviders(): ProviderListResponse;

  @route("/providers")
  @post
  createProvider(@body body: CreateProviderRequest): Provider;

  @route("/providers/{id}")
  @patch
  updateProvider(@path id: string, @body body: UpdateProviderRequest): Provider;

  @route("/resolvers")
  @get
  listResolverAccounts(): ResolverAccountListResponse;

  @route("/resolvers")
  @post
  createResolverAccount(@body body: CreateResolverAccountRequest): ResolverAccount;

  @route("/resolvers/{id}")
  @patch
  updateResolverAccount(@path id: string, @body body: UpdateResolverAccountRequest): ResolverAccount;

  @route("/installations")
  @get
  listInstallations(): InstallationListResponse;

  @route("/installations")
  @post
  createInstallation(@body body: CreateInstallationRequest): Installation;

  @route("/installations/{id}")
  @patch
  updateInstallation(@path id: string, @body body: UpdateInstallationRequest): Installation;

  @route("/installations/{id}/rotate-token")
  @post
  rotateInstallationToken(@path id: string): Installation;
}
