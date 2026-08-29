<script lang="ts">
	import Secrets from '#lib/Secrets.svelte';
	import { access } from '#lib/access.svelte.js';
</script>

<Secrets scopes={access.config.scopes} />
