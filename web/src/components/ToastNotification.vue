<script setup lang="ts">
import { ref, watch } from "vue";

export interface Toast {
  id: number;
  message: string;
  type: "success" | "error" | "info";
}

const toasts = ref<Toast[]>([]);
let nextId = 0;

function addToast(message: string, type: Toast["type"] = "info", durationMs = 4000) {
  const id = nextId++;
  toasts.value.push({ id, message, type });
  setTimeout(() => {
    removeToast(id);
  }, durationMs);
}

function removeToast(id: number) {
  toasts.value = toasts.value.filter((t) => t.id !== id);
}

// Expose addToast for external usage
defineExpose({ addToast });

// Also make it available via a global event pattern
watch(
  () => toasts.value.length,
  () => {
    // Reactive trigger for the template
  }
);
</script>

<template>
  <div class="toast-container">
    <div
      v-for="toast in toasts"
      :key="toast.id"
      class="toast"
      :class="`toast-${toast.type}`"
      @click="removeToast(toast.id)"
    >
      {{ toast.message }}
    </div>
  </div>
</template>

<style scoped>
.toast-container {
  position: fixed;
  top: 1rem;
  right: 1rem;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-width: 24rem;
}

.toast {
  padding: 0.75rem 1rem;
  border-radius: 0.375rem;
  font-size: 0.875rem;
  cursor: pointer;
  box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1);
  animation: toast-in 0.2s ease-out;
}

.toast-success {
  background: var(--color-badge-success-bg, #dcfce7);
  color: var(--color-badge-success-text, #166534);
  border: 1px solid #bbf7d0;
}

.toast-error {
  background: var(--color-badge-danger-bg, #fee2e2);
  color: var(--color-badge-danger-text, #991b1b);
  border: 1px solid #fecaca;
}

.toast-info {
  background: var(--color-badge-info-bg, #dbeafe);
  color: var(--color-badge-info-text, #1e40af);
  border: 1px solid #bfdbfe;
}

@keyframes toast-in {
  from {
    opacity: 0;
    transform: translateY(-0.5rem);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
