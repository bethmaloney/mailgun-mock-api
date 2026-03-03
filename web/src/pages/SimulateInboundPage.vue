<script setup lang="ts">
import { ref, onMounted, computed, watch } from "vue";
import { api } from "@/api/client";
import StatusBadge from "@/components/StatusBadge.vue";

// --- Types ---

interface Domain {
  name: string;
  state: string;
}

interface DomainsResponse {
  items: Domain[];
  total_count: number;
}

interface InboundResponse {
  message: string;
  matched_routes: string[];
  actions_executed: string[];
}

// --- Domain selector state ---

const domains = ref<Domain[]>([]);
const selectedDomain = ref("");
const domainsLoading = ref(true);
const error = ref<string | null>(null);

// --- Form state ---

const fromEmail = ref("");
const toEmail = ref("");
const subject = ref("");
const bodyPlain = ref("");
const bodyHtml = ref("");

// --- Submission state ---

const submitting = ref(false);
const result = ref<InboundResponse | null>(null);
const submitError = ref<string | null>(null);

// --- Computed ---

const canSubmit = computed(() => {
  return (
    selectedDomain.value &&
    fromEmail.value.trim() &&
    toEmail.value.trim() &&
    subject.value.trim() &&
    !submitting.value
  );
});

const hasMatchedRoutes = computed(() => {
  return result.value && result.value.matched_routes && result.value.matched_routes.length > 0;
});

// --- API functions ---

async function fetchDomains() {
  domainsLoading.value = true;
  error.value = null;
  try {
    const resp = await api.get<DomainsResponse>("/v4/domains");
    domains.value = resp.items || [];
    if (domains.value.length > 0 && !selectedDomain.value) {
      selectedDomain.value = domains.value[0].name;
    }
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load domains";
  } finally {
    domainsLoading.value = false;
  }
}

function onDomainChange() {
  result.value = null;
  submitError.value = null;
}

async function submitInbound() {
  if (!canSubmit.value) return;

  submitting.value = true;
  result.value = null;
  submitError.value = null;

  try {
    const resp = await api.post<InboundResponse>(
      `/mock/inbound/${encodeURIComponent(selectedDomain.value)}`,
      {
        from: fromEmail.value.trim(),
        recipient: toEmail.value.trim(),
        subject: subject.value.trim(),
        "body-plain": bodyPlain.value,
        "body-html": bodyHtml.value,
      }
    );
    result.value = resp;
  } catch (e: unknown) {
    const err = e as { message?: string };
    submitError.value = err.message || "Failed to simulate inbound message";
  } finally {
    submitting.value = false;
  }
}

function resetForm() {
  fromEmail.value = "";
  toEmail.value = "";
  subject.value = "";
  bodyPlain.value = "";
  bodyHtml.value = "";
  result.value = null;
  submitError.value = null;
}

// --- Watchers ---

watch(selectedDomain, (newDomain) => {
  // Auto-populate the "To" field suffix when domain changes
  if (newDomain && !toEmail.value) {
    toEmail.value = `recipient@${newDomain}`;
  } else if (newDomain && toEmail.value) {
    // Update the domain part if the current value ends with a domain-like suffix
    const atIndex = toEmail.value.lastIndexOf("@");
    if (atIndex >= 0) {
      const localPart = toEmail.value.substring(0, atIndex);
      toEmail.value = `${localPart}@${newDomain}`;
    }
  }
});

// --- Lifecycle ---

onMounted(() => {
  fetchDomains();
});
</script>

<template>
  <div class="page">
    <h1>Simulate Inbound</h1>

    <!-- Inbound Email Form -->
    <div class="card-section">
      <div class="section-header">
        <h2>Inbound Email</h2>
      </div>

      <!-- Domain Selector -->
      <div class="controls">
        <div class="domain-selector">
          <label for="inbound-domain-select">Domain:</label>
          <select
            id="inbound-domain-select"
            v-model="selectedDomain"
            class="select-input"
            @change="onDomainChange"
          >
            <option
              v-if="domainsLoading"
              value=""
            >
              Loading domains...
            </option>
            <option
              v-else-if="domains.length === 0"
              value=""
            >
              No domains available
            </option>
            <option
              v-for="d in domains"
              :key="d.name"
              :value="d.name"
            >
              {{ d.name }}
            </option>
          </select>
        </div>
      </div>

      <div
        v-if="error"
        class="error"
      >
        {{ error }}
      </div>

      <div
        v-if="!selectedDomain && !domainsLoading"
        class="info"
      >
        No domains configured. Create a domain first to simulate inbound messages.
      </div>

      <template v-if="selectedDomain">
        <!-- Email Form Fields -->
        <div class="form-grid">
          <div class="form-field">
            <label for="inbound-from">From <span class="required">*</span></label>
            <input
              id="inbound-from"
              v-model="fromEmail"
              type="email"
              class="filter-input"
              placeholder="sender@example.com"
            >
          </div>
          <div class="form-field">
            <label for="inbound-to">To <span class="required">*</span></label>
            <input
              id="inbound-to"
              v-model="toEmail"
              type="email"
              class="filter-input"
              :placeholder="`recipient@${selectedDomain}`"
            >
          </div>
          <div class="form-field form-field-full">
            <label for="inbound-subject">Subject <span class="required">*</span></label>
            <input
              id="inbound-subject"
              v-model="subject"
              type="text"
              class="filter-input"
              placeholder="Email subject line"
            >
          </div>
          <div class="form-field form-field-full">
            <label for="inbound-body-plain">Body Plain</label>
            <textarea
              id="inbound-body-plain"
              v-model="bodyPlain"
              class="textarea-input"
              placeholder="Plain text body content"
              rows="4"
            />
          </div>
          <div class="form-field form-field-full">
            <label for="inbound-body-html">Body HTML</label>
            <textarea
              id="inbound-body-html"
              v-model="bodyHtml"
              class="textarea-input"
              placeholder="<html><body><p>HTML body content</p></body></html>"
              rows="4"
            />
          </div>
        </div>

        <!-- Action Buttons -->
        <div class="form-actions">
          <button
            class="btn btn-primary btn-send"
            :disabled="!canSubmit"
            @click="submitInbound"
          >
            {{ submitting ? "Sending..." : "Simulate Inbound" }}
          </button>
          <button
            class="btn btn-sm"
            @click="resetForm"
          >
            Reset
          </button>
        </div>
      </template>
    </div>

    <!-- Error Panel -->
    <div
      v-if="submitError"
      class="card-section"
    >
      <div class="trigger-result result-error">
        <div class="result-header">
          <StatusBadge status="failed" />
          <span class="result-label">Error</span>
        </div>
        <p class="result-message">
          {{ submitError }}
        </p>
      </div>
    </div>

    <!-- Result Panel -->
    <div
      v-if="result"
      class="card-section"
    >
      <div class="section-header">
        <h2>Simulation Result</h2>
      </div>

      <!-- Response Message -->
      <div class="result-event-id">
        <label>Message</label>
        <span>{{ result.message }}</span>
      </div>

      <!-- Matched Routes -->
      <div class="detail-section">
        <h3>Matched Routes</h3>

        <div
          v-if="!hasMatchedRoutes"
          class="info"
        >
          No routes matched this inbound message. Create routes with matching expressions to handle inbound mail.
        </div>

        <ul
          v-else
          class="actions-list"
        >
          <li
            v-for="(route, idx) in result.matched_routes"
            :key="idx"
          >
            <code class="action-item">{{ route }}</code>
          </li>
        </ul>
      </div>

      <!-- Actions Executed -->
      <div
        v-if="result.actions_executed && result.actions_executed.length > 0"
        class="detail-section"
      >
        <h3>Actions Executed</h3>

        <ul class="actions-list">
          <li
            v-for="(action, idx) in result.actions_executed"
            :key="idx"
          >
            <code class="action-item">{{ action }}</code>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>

<style scoped>
.controls {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  align-items: flex-end;
  margin-bottom: 1rem;
}

.domain-selector {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.domain-selector label {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--color-text-secondary, #64748b);
}

.select-input {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  background: var(--color-bg-primary, #ffffff);
  color: var(--color-text-primary, #1e293b);
  min-width: 10rem;
}

.select-input:focus {
  outline: 2px solid var(--color-primary, #3b82f6);
  outline-offset: -1px;
}

.filter-input {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  background: var(--color-bg-primary, #ffffff);
  color: var(--color-text-primary, #1e293b);
  width: 100%;
  box-sizing: border-box;
}

.filter-input:focus {
  outline: 2px solid var(--color-primary, #3b82f6);
  outline-offset: -1px;
}

.textarea-input {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  background: var(--color-bg-primary, #ffffff);
  color: var(--color-text-primary, #1e293b);
  width: 100%;
  box-sizing: border-box;
  font-family: monospace;
  resize: vertical;
}

.textarea-input:focus {
  outline: 2px solid var(--color-primary, #3b82f6);
  outline-offset: -1px;
}

.btn {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  cursor: pointer;
  transition: background-color 0.15s;
}

.btn-primary {
  background: var(--color-primary, #3b82f6);
  color: #ffffff;
  border-color: var(--color-primary, #3b82f6);
}

.btn-primary:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-sm {
  padding: 0.25rem 0.5rem;
  font-size: 0.75rem;
  background: var(--color-bg-primary, #ffffff);
}

.btn-sm:hover {
  background: var(--color-bg-hover, #f1f5f9);
}

.btn-send {
  padding: 0.5rem 1.5rem;
  font-size: 1rem;
  font-weight: 600;
}

.error {
  padding: 0.75rem 1rem;
  margin-bottom: 1rem;
  background: var(--color-badge-danger-bg, #fee2e2);
  color: var(--color-badge-danger-text, #991b1b);
  border-radius: 0.375rem;
}

.info {
  padding: 0.75rem 1rem;
  margin-bottom: 1rem;
  background: var(--color-badge-info-bg, #dbeafe);
  color: var(--color-badge-info-text, #1e40af);
  border-radius: 0.375rem;
}

.card-section {
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.5rem;
  padding: 1.5rem;
  margin-bottom: 1.5rem;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.section-header h2 {
  margin: 0;
  font-size: 1.125rem;
}

.mono {
  font-family: monospace;
  font-size: 0.8125rem;
  word-break: break-all;
}

/* Form layout */
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.form-field label {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
}

.form-field-full {
  grid-column: 1 / -1;
}

.required {
  color: var(--color-badge-danger-text, #991b1b);
}

.form-actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

/* Result sections */
.result-event-id {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 1rem;
  padding: 0.75rem 1rem;
  background: var(--color-bg-subtle, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
}

.result-event-id label {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
  white-space: nowrap;
}

.detail-section {
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--color-border, #e2e8f0);
}

.detail-section h3 {
  font-size: 0.875rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
  margin: 0 0 0.75rem;
}

.detail-field {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.detail-field label {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
}

.detail-field span {
  font-size: 0.875rem;
  color: var(--color-text-primary, #1e293b);
  word-break: break-all;
}

/* Routes list */
.routes-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.route-card {
  background: var(--color-bg-subtle, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  padding: 1rem;
}

.route-header {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  margin-bottom: 0.5rem;
}

.route-header label {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
}

.route-expression {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  margin-bottom: 0.5rem;
}

.route-expression label {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
}

.expression-code {
  display: inline-block;
  padding: 0.25rem 0.625rem;
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  font-size: 0.8125rem;
  font-family: monospace;
  color: var(--color-text-primary, #1e293b);
  word-break: break-all;
}

.route-actions label {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-text-secondary, #64748b);
  display: block;
  margin-bottom: 0.25rem;
}

.actions-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.actions-list li {
  padding: 0;
}

.action-item {
  display: inline-block;
  padding: 0.25rem 0.625rem;
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  font-size: 0.8125rem;
  font-family: monospace;
  color: var(--color-text-primary, #1e293b);
}

/* Actions taken */
.actions-taken-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.action-taken-card {
  background: var(--color-bg-subtle, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  padding: 1rem;
}

.action-taken-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}

.action-taken-type {
  font-weight: 600;
  font-size: 0.875rem;
  text-transform: capitalize;
  color: var(--color-text-primary, #1e293b);
}

.action-taken-details {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
  gap: 0.75rem;
}

/* Error/success result panels */
.trigger-result {
  border-radius: 0.375rem;
  padding: 1rem;
}

.result-error {
  background: var(--color-badge-danger-bg, #fee2e2);
  border: 1px solid var(--color-badge-danger-text, #991b1b);
}

.result-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}

.result-label {
  font-weight: 600;
  font-size: 0.875rem;
}

.result-message {
  margin: 0;
  font-size: 0.875rem;
  word-break: break-word;
}
</style>
