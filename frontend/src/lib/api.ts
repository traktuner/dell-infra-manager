import type {
	Server,
	ServerCache,
	Dashboard,
	ThermalInfo,
	PowerInfo,
	FirmwareComponent,
	AvailableUpdate,
	Job,
	IDRACJob,
	StorageDetail,
	BiosAttributes,
	BiosRegistryEntry,
	VirtualMedia,
	LogEntry,
	ResetType
} from './types';

const BASE = '/api/v1';

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
	const res = await fetch(BASE + path, {
		method,
		headers: body ? { 'Content-Type': 'application/json' } : {},
		body: body ? JSON.stringify(body) : undefined
	});
	if (!res.ok) {
		const err = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(err.error ?? res.statusText);
	}
	if (res.status === 204) return undefined as T;
	return res.json();
}

// Servers
export const api = {
	servers: {
		list: () => request<Server[]>('GET', '/servers'),
		get: (id: string) => request<Server>('GET', `/servers/${id}`),
		create: (data: unknown) => request<Server>('POST', '/servers', data),
		update: (id: string, data: unknown) => request<Server>('PUT', `/servers/${id}`, data),
		delete: (id: string) => request<void>('DELETE', `/servers/${id}`),
		// Stateless: tests credentials in body, nothing persisted to DB.
		testCredentials: (data: unknown) =>
			request<{ ok: boolean; error?: string }>('POST', '/servers/test', data),
		// For an existing server: tests with stored credentials.
		test: (id: string) => request<{ ok: boolean; error?: string }>('POST', `/servers/${id}/test`)
	},
	cache: {
		summary: (id: string) => request<ServerCache>('GET', `/servers/${id}/summary`),
		thermal: (id: string) => request<ThermalInfo>('GET', `/servers/${id}/thermal`),
		power: (id: string) => request<PowerInfo>('GET', `/servers/${id}/power`),
		storage: (id: string) => request<StorageDetail[]>('GET', `/servers/${id}/storage`),
		firmware: (id: string) => request<FirmwareComponent[]>('GET', `/servers/${id}/firmware`)
	},
	power: {
		action: (id: string, action: ResetType) =>
			request<{ ok: boolean }>('POST', `/servers/${id}/power`, { action }),
		bulk: (serverIds: string[], action: ResetType) =>
			request<unknown[]>('POST', '/servers/bulk/power', { server_ids: serverIds, action })
	},
	bios: {
		get: (id: string) => request<BiosAttributes>('GET', `/servers/${id}/bios`),
		registry: (id: string) =>
			request<{ RegistryEntries: { Attributes: BiosRegistryEntry[] } }>('GET', `/servers/${id}/bios/registry`),
		patch: (id: string, attributes: Record<string, unknown>, applyTime = 'OnReset') =>
			request<{ job_id: string }>('PATCH', `/servers/${id}/bios/settings`, {
				attributes,
				apply_time: applyTime
			}),
		pending: (id: string) => request<Job[]>('GET', `/servers/${id}/bios/pending`)
	},
	virtualmedia: {
		status: (id: string) => request<VirtualMedia[]>('GET', `/servers/${id}/virtualmedia`),
		insert: (id: string, imageUrl: string, slot = 'CD') =>
			request<{ ok: boolean }>('POST', `/servers/${id}/virtualmedia/insert`, {
				image_url: imageUrl,
				slot
			}),
		eject: (id: string, slot = 'CD') =>
			request<{ ok: boolean }>('POST', `/servers/${id}/virtualmedia/eject`, { slot })
	},
	firmware: {
		inventory: (id: string) => request<FirmwareComponent[]>('GET', `/servers/${id}/firmware`),
		available: (id: string) => request<AvailableUpdate[]>('GET', `/servers/${id}/firmware/available`),
		update: (id: string, component: string, catalogPath: string, version?: string) =>
			request<{ job_id: string }>('POST', `/servers/${id}/firmware/update`, {
				component,
				catalog_path: catalogPath,
				version
			}),
		bulkUpdate: (serverIds: string[], component: string, catalogPath: string) =>
			request<unknown[]>('POST', '/servers/bulk/firmware/update', {
				server_ids: serverIds,
				component,
				catalog_path: catalogPath
			})
	},
	jobs: {
		all: () => request<Job[]>('GET', '/jobs'),
		forServer: (id: string) => request<Job[]>('GET', `/servers/${id}/jobs`),
		idrac: (id: string) => request<IDRACJob[]>('GET', `/servers/${id}/jobs/idrac`),
		delete: (serverId: string, jobId: string) =>
			request<void>('DELETE', `/servers/${serverId}/jobs/${jobId}`),
		clearAll: (serverId: string) => request<{ ok: boolean }>('DELETE', `/servers/${serverId}/jobs`)
	},
	eventlog: {
		get: (id: string, top = 100, skip = 0) =>
			request<{ Members: LogEntry[]; 'Members@odata.count': number }>(
				'GET',
				`/servers/${id}/eventlog?top=${top}&skip=${skip}`
			),
		clear: (id: string) => request<{ ok: boolean }>('DELETE', `/servers/${id}/eventlog`)
	},
	dashboard: () => request<Dashboard>('GET', '/dashboard')
};
