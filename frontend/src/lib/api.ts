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
		// `refresh=true` does a conditional GET against Dell's catalog server
		// before running the comparison — cheap if Dell hasn't published a
		// newer catalog (304), full re-download otherwise.
		available: (id: string, refresh = false) =>
			request<AvailableUpdate[]>(
				'GET',
				`/servers/${id}/firmware/available${refresh ? '?refresh=1' : ''}`
			),
		update: (id: string, update: AvailableUpdate) =>
			request<{ job_id: string }>('POST', `/servers/${id}/firmware/update`, {
				component: update.component,
				inventory_id: update.inventory_id,
				software_id: update.software_id,
				catalog_path: update.catalog_path,
				version: update.available_version,
				apply_time: 'OnReset'
			}),
		bulkUpdate: (serverIds: string[], component: string, catalogPath: string, version: string) =>
			request<unknown[]>('POST', '/servers/bulk/firmware/update', {
				server_ids: serverIds,
				component,
				catalog_path: catalogPath,
				version,
				apply_time: 'OnReset'
			})
	},
	catalog: {
		info: () =>
			request<{
				available: boolean;
				date_time?: string;
				version?: string;
				fetched_at?: string;
			}>('GET', '/catalog/info'),
		refresh: () =>
			request<{
				updated: boolean;
				date_time?: string;
				version?: string;
				fetched_at?: string;
			}>('POST', '/catalog/refresh')
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
	vnc: {
		/** Idempotent: returns port + token, configures iDRAC only if needed. */
		enable: (id: string) =>
			request<{ port: number; token: string; fallback?: string; error?: string }>(
				'POST',
				`/servers/${id}/vnc/enable`
			),
		/** Clears stored credentials so next enable rotates the iDRAC password. */
		reset: (id: string) => request<{ ok: boolean }>('POST', `/servers/${id}/vnc/reset`),
		/** Builds the WebSocket URL for the VNC proxy (not a fetch call). */
		proxyUrl: (id: string, token: string): string => {
			const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
			return `${proto}//${location.host}/api/v1/servers/${id}/vnc/proxy?token=${encodeURIComponent(token)}`;
		}
	},
	dashboard: () => request<Dashboard>('GET', '/dashboard'),
	auth: {
		me: () =>
			request<{
				user: { email: string; name?: string; groups?: string[] };
				auth_enabled: boolean;
			}>('GET', '/me')
	},
	settings: {
		getNotifications: () => request<NotificationSettings>('GET', '/settings/notifications'),
		updateNotifications: (data: NotificationSettingsInput) =>
			request<{ ok: boolean }>('PUT', '/settings/notifications', data),
		testNotifications: (data: NotificationSettingsInput) =>
			request<{ ok: boolean }>('POST', '/settings/notifications/test', data),
		/** Triggers the firmware-update digest immediately, ignoring the daily
		 *  schedule. Useful for verifying SMTP setup without waiting until
		 *  tomorrow morning. Sends only if there are outdated components. */
		sendDigestNow: () =>
			request<{ ok: boolean }>('POST', '/settings/notifications/digest-now')
	},
	appliance: {
		updateStatus: () => request<ApplianceUpdateStatus>('GET', '/appliance/update'),
		applyUpdate: () => request<{ updated: boolean; version: string; binary_sha256?: string }>('POST', '/appliance/update')
	}
};

export type ApplianceUpdateStatus = {
	supported: boolean;
	current_version: string;
	current_commit: string;
	current_sha256: string;
	repository: string;
	latest_version?: string;
	release_url?: string;
	update_available?: boolean;
	checked_at?: string;
	check_error?: string;
	active_firmware_jobs?: number;
};

export type NotificationSettings = {
	enabled: boolean;
	smtp_host: string;
	smtp_port: number;
	smtp_username: string;
	smtp_from: string;
	smtp_tls: 'none' | 'starttls' | 'tls';
	recipients: string;
	on_server_offline: boolean;
	on_health_critical: boolean;
	on_job_failed: boolean;
	on_firmware_updates: boolean;
	updated_at: string;
	has_password: boolean;
};

export type NotificationSettingsInput = Omit<NotificationSettings, 'updated_at' | 'has_password'> & {
	smtp_password?: string;
};
