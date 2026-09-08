<script setup lang="ts">
import type { Navigation } from '../types/domain'
defineProps<{ navigation: Navigation }>()
function openNavigation(navigation: Navigation) { window.location.href = navigation.uri || navigation.fallbackUrl || '#' }
</script>
<template><button class="navigate-button" type="button" @click="openNavigation(navigation)">立即导航</button></template>
