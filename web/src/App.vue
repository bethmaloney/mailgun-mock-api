<script setup lang="ts">
import { ref, onMounted } from "vue";

const healthStatus = ref<string>("loading...");

onMounted(async () => {
  try {
    const res = await fetch("/mock/health");
    const data = await res.json();
    healthStatus.value = data.status;
  } catch {
    healthStatus.value = "error";
  }
});
</script>

<template>
  <div id="app">
    <h1>Mailgun Mock API</h1>
    <p>Health: {{ healthStatus }}</p>
  </div>
</template>
