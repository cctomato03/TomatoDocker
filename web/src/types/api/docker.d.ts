/**
 * Docker 管理模块前端类型（对应后端 /api/v1/docker/** 出参）。
 */
declare namespace Docker {
  interface Environment {
    id: string
    name: string
    description: string
    type: 'local' | 'tcp' | 'tls' | 'ssh'
    host: string
    socketPath: string
    sshUser: string
    isDefault: boolean
    status: 'online' | 'offline' | 'unknown'
    statusMessage: string
    accessLevel: 0 | 1 | 2 | 3
    hasTls: boolean
    hasSshPassword: boolean
    hasSshKey: boolean
    swarmActive: boolean
    swarmRole: string
    createdAt: string
    updatedAt: string
  }

  interface EnvironmentReq {
    name: string
    description?: string
    type: string
    host?: string
    socketPath?: string
    tlsCa?: string
    tlsCert?: string
    tlsKey?: string
    sshUser?: string
    sshPassword?: string
    sshKey?: string
    isDefault?: boolean
    clearCredentials?: boolean
  }

  interface GrantOption {
    subjectType: 'role' | 'user'
    subjectId: string
    name: string
    detail: string
  }

  interface Grant {
    id: string
    subjectType: 'role' | 'user'
    subjectId: string
    name: string
    accessLevel: number
    createdAt: string
  }

  interface Port {
    ip: string
    privatePort: number
    publicPort: number
    type: string
  }

  interface Container {
    id: string
    name: string
    names: string[]
    image: string
    imageId: string
    command: string
    created: number
    state: string
    status: string
    ports: Port[]
    labels: Record<string, string>
    stackName: string
    serviceName: string
    projectName: string
    health: string
    composeMode: boolean
    swarmService: boolean
  }

  interface Image {
    id: string
    shortId: string
    repoTags: string[]
    repoDigests: string[]
    created: number
    size: number
    sharedSize: number
    containers: number
    labels: Record<string, string>
  }

  interface Network {
    id: string
    name: string
    driver: string
    scope: string
    internal: boolean
    attachable: boolean
    ingress: boolean
    enableIpv6: boolean
    subnets: string[]
    containers: number
    labels: Record<string, string>
    created: string
    stackName: string
  }

  interface Volume {
    name: string
    driver: string
    mountpoint: string
    scope: string
    createdAt: string
    labels: Record<string, string>
    stackName: string
    inUse: boolean
  }

  interface Node {
    id: string
    hostname: string
    role: string
    availability: string
    status: string
    addr: string
    version: string
    platformOs: string
    platformArch: string
    nCpu: number
    memTotal: number
    leader: boolean
    managerStatus: string
    labels: Record<string, string>
    engineLabels: Record<string, string>
    createdAt: string
    updatedAt: string
  }

  interface ServicePort {
    name: string
    protocol: string
    targetPort: number
    publishedPort: number
    publishMode: string
  }

  interface Service {
    id: string
    name: string
    image: string
    mode: string
    replicas: number
    running: number
    ports: ServicePort[]
    stackName: string
    labels: Record<string, string>
    command: string
    createdAt: string
    updatedAt: string
    updateStatus: string
  }

  interface Task {
    id: string
    name: string
    serviceId: string
    serviceName: string
    nodeId: string
    nodeName: string
    slot: number
    state: string
    desiredState: string
    message: string
    err: string
    createdAt: string
    updatedAt: string
  }

  interface Secret {
    id: string
    name: string
    labels: Record<string, string>
    driver: string
    createdAt: string
    updatedAt: string
    stackName: string
  }

  interface ConfigObj {
    id: string
    name: string
    labels: Record<string, string>
    createdAt: string
    updatedAt: string
    stackName: string
  }

  interface SwarmState {
    active: boolean
    nodeId: string
    nodeAddr: string
    localNodeState: string
    controlAvailable: boolean
    error: string
    clusterId: string
    name: string
    labels: Record<string, string>
    nodes: number
    managers: number
    createdAt: string
    updatedAt: string
  }

  interface RuntimeResource {
    id: string
    name: string
    kind: 'container' | 'service'
    state: string
    image: string
    detail: string
  }

  interface StackRuntime {
    status: string
    containersTotal: number
    containersRunning: number
    servicesTotal: number
    servicesRunning: number
    resources: RuntimeResource[]
  }

  interface Stack {
    id: string
    environmentId: string
    environmentName: string
    name: string
    type: 'compose' | 'swarm'
    status: 'active' | 'inactive' | 'error' | 'deploying'
    statusMessage: string
    version: number
    lastDeployedAt?: string | null
    createdBy: string
    updatedBy: string
    createdAt: string
    updatedAt: string
    accessLevel: number
    pendingChanges: boolean
    runtime: StackRuntime
    runtimeError?: string
  }

  interface Revision {
    id: string
    stackId: string
    version: number
    composeContent: string
    envContent: string
    message: string
    createdBy: string
    createdAt: string
  }

  interface Activity {
    id: string
    stackId: string
    action: string
    status: 'success' | 'failed'
    message: string
    operator: string
    createdAt: string
  }

  interface StackDetail extends Stack {
    composeContent: string
    envContent: string
    revisions: Revision[]
    activities: Activity[]
  }

  interface Info {
    name: string
    serverVersion: string
    apiVersion: string
    operatingSystem: string
    osType: string
    architecture: string
    kernelVersion: string
    nCpu: number
    memTotal: number
    containers: number
    running: number
    paused: number
    stopped: number
    images: number
    driver: string
    dockerRootDir: string
    loggingDriver: string
    cgroupDriver: string
    swarmActive: boolean
    swarmNodeId: string
    swarmNodeAddr: string
    swarmRole: string
    swarmNodes: number
    swarmManagers: number
    warnings?: string[]
  }

  interface DiskUsage {
    layersSize: number
    imagesSize: number
    volumesSize: number
    buildCache: number
    images: number
    containers: number
    volumes: number
  }

  interface Overview {
    info: Info
    usage: DiskUsage
    networks: number
    stacks: number
    services: number
    tasks: number
    secrets: number
    configs: number
    nodes: number
    stackList: Stack[]
  }

  interface ContainerStats {
    cpuPercent: number
    memUsage: number
    memLimit: number
    memPercent: number
    netRx: number
    netTx: number
    blockRead: number
    blockWrite: number
    pids: number
  }

  interface StackExport {
    environmentName: string
    name: string
    type: string
    composeContent: string
    envContent: string
  }
}