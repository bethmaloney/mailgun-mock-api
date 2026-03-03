<script setup lang="ts">
defineProps<{
  hasNext: boolean;
  hasPrevious: boolean;
}>();

const emit = defineEmits<{
  next: [];
  previous: [];
}>();
</script>

<template>
  <div class="pagination">
    <button
      class="pagination-btn"
      :disabled="!hasPrevious"
      @click="emit('previous')"
    >
      Previous
    </button>
    <button
      class="pagination-btn"
      :disabled="!hasNext"
      @click="emit('next')"
    >
      Next
    </button>
  </div>
</template>

<style scoped>
.pagination {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  padding: 0.75rem 0;
}

.pagination-btn {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  background: var(--color-bg-primary, #ffffff);
  color: var(--color-text-primary, #1e293b);
  cursor: pointer;
  transition: background-color 0.15s, border-color 0.15s;
}

.pagination-btn:hover:not(:disabled) {
  background: var(--color-bg-hover, #f1f5f9);
  border-color: var(--color-border-hover, #cbd5e1);
}

.pagination-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
