export interface Server {
	id: string;
	name: string;
	hostname: string;
	port: number;
	username: string;
	tls_verify: boolean;
	tags: string; // JSON array string
	created_at: string;
	updated_at: string;
}

export interface ServerCache {
	server_id: string;
	system_json: string | null;
	thermal_json: string | null;
	power_json: string | null;
	firmware_json: string | null;
	storage_json: string | null;
	bios_json: string | null;
	virtualmedia_json: string | null;
	last_seen: string | null;
	status: 'online' | 'offline' | 'error' | 'unknown';
}

export interface SystemInfo {
	Id: string;
	Name: string;
	Model: string;
	SerialNumber: string;
	HostName: string;
	PowerState: 'On' | 'Off' | 'PoweringOn' | 'PoweringOff';
	BiosVersion: string;
	Status: { Health: string; State: string };
	ProcessorSummary: { Count: number; LogicalProcessorCount?: number; Model?: string };
	MemorySummary: { TotalSystemMemoryGiB: number };
	// Enriched server-side from /Memory and /Processors collections.
	MemoryType?: string;
	MemorySpeedMHz?: number;
	CoresPerCPU?: number;
	ThreadsPerCPU?: number;
}

export interface ThermalInfo {
	Temperatures: TempSensor[];
	Fans: FanSensor[];
}

export interface TempSensor {
	Name: string;
	ReadingCelsius: number;
	UpperThresholdCritical: number;
	Status: { Health: string; State: string };
}

export interface FanSensor {
	Name: string;
	Reading: number;
	ReadingUnits: string;
	Status: { Health: string; State: string };
}

export interface PowerInfo {
	PowerControl: PowerControl[];
	PowerSupplies: PowerSupply[];
}

export interface PowerControl {
	Name: string;
	PowerConsumedWatts: number;
	PowerCapacityWatts: number;
}

export interface PowerSupply {
	Name: string;
	LastPowerOutputWatts: number;
	LineInputVoltage: number;
	Status: { Health: string; State: string };
}

export interface FirmwareComponent {
	Id: string;
	Name: string;
	Version: string;
	Updateable: boolean;
	Status: { Health: string; State: string };
}

export interface AvailableUpdate {
	component: string;
	installed_version: string;
	available_version: string;
	release_date: string;
	catalog_path: string;
}

export interface Job {
	id: string;
	server_id: string;
	type: 'firmware_update' | 'bios_config';
	status: 'queued' | 'running' | 'done' | 'failed';
	payload: string;
	result: string | null;
	idrac_job_id: string | null;
	created_at: string;
	started_at: string | null;
	finished_at: string | null;
}

// Live job from iDRAC's own job queue (not our DB).
export interface IDRACJob {
	Id: string;
	Name: string;
	JobState: string;
	PercentComplete: number;
	Message: string;
	StartTime: string;
	EndTime: string;
}

export interface StorageController {
	Id: string;
	Name: string;
	Status: { Health: string; State: string };
}

export interface Drive {
	Id: string;
	Name: string;
	MediaType: string;
	CapacityBytes: number;
	Protocol: string;
	RotationSpeedRPM: number;
	FailurePredicted: boolean;
	Status: { Health: string; State: string };
}

export interface Volume {
	Id: string;
	Name: string;
	RAIDType: string;
	CapacityBytes: number;
	VolumeType: string;
	Status: { Health: string; State: string };
}

export interface StorageDetail {
	Controller: StorageController;
	Drives: Drive[];
	Volumes: Volume[];
}

export interface BiosAttributes {
	Attributes: Record<string, unknown>;
}

export interface BiosRegistryEntry {
	AttributeName: string;
	DisplayName: string;
	Type: 'String' | 'Integer' | 'Enumeration' | 'Boolean';
	ReadOnly: boolean;
	Value?: { ValueName: string }[];
	LowerBound?: number;
	UpperBound?: number;
	// Set client-side after fetching the registry, not part of the wire format:
	current_value?: unknown;
	AllowedValues?: (string | number)[];
}

export interface VirtualMedia {
	Id: string;
	MediaTypes: string[];
	Inserted: boolean;
	Image: string;
	ConnectedVia: string;
	WriteProtected: boolean;
	Status: { Health: string; State: string };
}

export interface Dashboard {
	total_servers: number;
	online: number;
	offline: number;
	error: number;
	active_jobs: number;
}

export interface LogEntry {
	id: string;
	created: string;
	severity: string;
	message: string;
	message_id: string;
	category: string;
}

export type ResetType =
	| 'On'
	| 'ForceOff'
	| 'GracefulShutdown'
	| 'GracefulRestart'
	| 'ForceRestart'
	| 'PushPowerButton'
	| 'Nmi';

// WebSocket event types
export interface WSEvent {
	type: string;
	server_id?: string;
	job_id?: string;
	data: Record<string, unknown>;
}
