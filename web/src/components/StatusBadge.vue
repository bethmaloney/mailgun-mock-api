<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  status: string;
  type?: string;
}>();

const badgeClass = computed(() => {
  const s = props.status.toLowerCase();

  // Event types
  if (s === "delivered" || s === "accepted" || s === "active") return "badge-success";
  if (s === "opened" || s === "clicked") return "badge-info";
  if (s === "failed" || s === "complained" || s === "disabled") return "badge-danger";
  if (s === "unsubscribed") return "badge-warning";
  if (s === "unverified") return "badge-warning";

  return "badge-default";
});
</script>

<template>
  <span
    class="status-badge"
    :class="badgeClass"
  >
    {{ status }}
  </span>
</template>

<style scoped>
.status-badge {
  display: inline-block;
  padding: 0.125rem 0.5rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: capitalize;
  line-height: 1.5;
}

.badge-success {
  background: var(--color-badge-success-bg, #dcfce7);
  color: var(--color-badge-success-text, #166534);
}

.badge-info {
  background: var(--color-badge-info-bg, #dbeafe);
  color: var(--color-badge-info-text, #1e40af);
}

.badge-danger {
  background: var(--color-badge-danger-bg, #fee2e2);
  color: var(--color-badge-danger-text, #991b1b);
}

.badge-warning {
  background: var(--color-badge-warning-bg, #fef3c7);
  color: var(--color-badge-warning-text, #92400e);
}

.badge-default {
  background: var(--color-badge-default-bg, #f1f5f9);
  color: var(--color-badge-default-text, #475569);
}
</style>
