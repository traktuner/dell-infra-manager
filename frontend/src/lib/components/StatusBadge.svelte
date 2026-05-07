<script lang="ts">
	type Props = { status: string; size?: 'sm' | 'md' };
	let { status, size = 'md' }: Props = $props();

	const colorMap: Record<string, string> = {
		online: 'bg-emerald-500/20 text-emerald-400 ring-emerald-500/30',
		offline: 'bg-red-500/20 text-red-400 ring-red-500/30',
		error: 'bg-orange-500/20 text-orange-400 ring-orange-500/30',
		unknown: 'bg-zinc-500/20 text-zinc-400 ring-zinc-500/30',
		On: 'bg-emerald-500/20 text-emerald-400 ring-emerald-500/30',
		Off: 'bg-zinc-500/20 text-zinc-400 ring-zinc-500/30',
		PoweringOn: 'bg-yellow-500/20 text-yellow-400 ring-yellow-500/30',
		PoweringOff: 'bg-yellow-500/20 text-yellow-400 ring-yellow-500/30',
		OK: 'bg-emerald-500/20 text-emerald-400 ring-emerald-500/30',
		Critical: 'bg-red-500/20 text-red-400 ring-red-500/30',
		Warning: 'bg-yellow-500/20 text-yellow-400 ring-yellow-500/30',
		queued: 'bg-zinc-500/20 text-zinc-400 ring-zinc-500/30',
		running: 'bg-blue-500/20 text-blue-400 ring-blue-500/30',
		done: 'bg-emerald-500/20 text-emerald-400 ring-emerald-500/30',
		failed: 'bg-red-500/20 text-red-400 ring-red-500/30'
	};

	const sizeClass = size === 'sm' ? 'px-1.5 py-0.5 text-xs' : 'px-2 py-1 text-xs';
	const color = colorMap[status] ?? 'bg-zinc-500/20 text-zinc-400 ring-zinc-500/30';
</script>

<span class="inline-flex items-center rounded-md ring-1 ring-inset font-medium {sizeClass} {color}">
	{status}
</span>
