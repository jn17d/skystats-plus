<script>
    import { onMount } from 'svelte';
    import { IconTrophy } from '@tabler/icons-svelte';
    import { refreshLeaderboardData } from '../stores/settings';

    let endpoint = 'api/stats/seen/leaderboard';

    let data = [];
    let loading = true;
    let error = null;

    async function fetchData() {
        try {
            const response = await fetch(endpoint);
            if (!response.ok) {
                throw new Error(`${response.status}`);
            }
            const result = await response.json();
            data = result;
            error = null;
        } catch (err) {
            error = err.message;
        } finally {
            loading = false;
        }
    }

    function formatDateTime(value) {
        return value ? new Date(value).toLocaleString() : '-';
    }

    onMount(() => {
        fetchData();
    })

    // Refresh when settings change
    $: if ($refreshLeaderboardData) {
        fetchData();
    }
</script>

<div>
    <div class="card bg-base-100 mb-4 w96 shadow-sm rounded hover:shadow-md transition-all duration-200">
        <div class="card-body">
            <div class="overflow-x-auto">
                {#if loading}
                    <div class="flex justify-center py-8">
                        <span class="loading loading-ring loading-lg"></span>
                    </div>
                {:else if error}
                    <div class="flex alert alert-error">
                        <svg xmlns="http://www.w3.org/2000/svg" class="stroke-current shrink-0 h-6 w-6" fill="none" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                        <span>Something went wrong: {error}</span>
                    </div>
                {:else if data.length === 0}
                    <div class="alert alert-info">
                        <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="stroke-current shrink-0 w-6 h-6"><path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                        <span>No data available</span>
                    </div>
                {:else}
                    <!-- table header -->
                    <div class="flex items-center gap-2 mb-2">
                        <div class="w-8 h-8 rounded-lg flex items-center justify-center">
                            <IconTrophy class="w-6 h-6 text-primary" />
                        </div>
                        <h2 class="text-2xl font-extralight tracking-wider">Most Seen Aircraft</h2>
                    </div>
                    <p class="text-sm text-base-content/70 mb-5">
                        Each sighting is a separate visit - a new visit is recorded when an aircraft is seen again after a gap of 10 minutes or more.
                    </p>
                    <!-- table -->
                    <table class="table">
                        <thead>
                            <tr class="uppercase tracking-wider">
                                <th>#</th>
                                <th>Reg</th>
                                <th>Type</th>
                                <th>Operator</th>
                                <th>Times Seen</th>
                                <th>Days Seen</th>
                                <th>First Seen</th>
                                <th>Last Seen</th>
                            </tr>
                        </thead>
                        <tbody>
                            {#each data as aircraft}
                            <tr>
                                <td>
                                    {#if aircraft.rank <= 3}
                                        <span class="badge badge-primary badge-sm">{aircraft.rank}</span>
                                    {:else}
                                        <span class="opacity-50">{aircraft.rank}</span>
                                    {/if}
                                </td>
                                <td class="font-mono whitespace-nowrap">{aircraft.registration || '-'}</td>
                                <td>{aircraft.type || aircraft.icao_type || '-'}</td>
                                <td>{aircraft.operator || '-'}</td>
                                <td>{aircraft.times_seen.toLocaleString()}</td>
                                <td>{aircraft.days_seen.toLocaleString()}</td>
                                <td class="whitespace-nowrap">{formatDateTime(aircraft.first_seen)}</td>
                                <td class="whitespace-nowrap">{formatDateTime(aircraft.last_seen)}</td>
                            </tr>
                            {/each}
                        </tbody>
                    </table>
                {/if}
            </div>
        </div>
    </div>
</div>
