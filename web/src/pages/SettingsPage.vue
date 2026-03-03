<script setup lang="ts">
import { ref, onMounted } from "vue";
import { api } from "@/api/client";

// --- Types ---

interface EventGeneration {
  auto_deliver: boolean;
  delivery_delay_ms: number;
  default_delivery_status_code: number;
  auto_fail_rate: number;
}

interface DomainBehavior {
  domain_auto_verify: boolean;
  sandbox_domain: string;
}

interface WebhookDelivery {
  webhook_retry_mode: string;
  webhook_timeout_ms: number;
}

interface Authentication {
  auth_mode: string;
  signing_key: string;
}

interface Storage {
  store_attachment_bytes: boolean;
  max_messages: number;
  max_events: number;
}

interface MockConfig {
  event_generation: EventGeneration;
  domain_behavior: DomainBehavior;
  webhook_delivery: WebhookDelivery;
  authentication: Authentication;
  storage: Storage;
}

interface Domain {
  name: string;
  state: string;
}

interface DomainsResponse {
  items: Domain[];
}

interface ResetResponse {
  message: string;
}

// --- State ---

const loading = ref(true);
const error = ref<string | null>(null);
const successMessage = ref<string | null>(null);
let successTimer: ReturnType<typeof setTimeout> | null = null;

// Config section state
const eventGeneration = ref<EventGeneration>({
  auto_deliver: true,
  delivery_delay_ms: 0,
  default_delivery_status_code: 250,
  auto_fail_rate: 0.0,
});

const domainBehavior = ref<DomainBehavior>({
  domain_auto_verify: true,
  sandbox_domain: "",
});

const webhookDelivery = ref<WebhookDelivery>({
  webhook_retry_mode: "immediate",
  webhook_timeout_ms: 5000,
});

const authentication = ref<Authentication>({
  auth_mode: "accept_any",
  signing_key: "",
});

const storage = ref<Storage>({
  store_attachment_bytes: false,
  max_messages: 0,
  max_events: 0,
});

// Section saving state
const savingEventGeneration = ref(false);
const savingDomainBehavior = ref(false);
const savingWebhookDelivery = ref(false);
const savingAuthentication = ref(false);
const savingStorage = ref(false);

// Reset state
const domains = ref<Domain[]>([]);
const domainsLoading = ref(false);
const resetDomain = ref("");
const resetting = ref(false);

// --- Functions ---

function showSuccess(message: string) {
  successMessage.value = message;
  if (successTimer) {
    clearTimeout(successTimer);
  }
  successTimer = setTimeout(() => {
    successMessage.value = null;
  }, 3000);
}

function applyConfig(config: MockConfig) {
  eventGeneration.value = { ...config.event_generation };
  domainBehavior.value = { ...config.domain_behavior };
  webhookDelivery.value = { ...config.webhook_delivery };
  authentication.value = { ...config.authentication };
  storage.value = { ...config.storage };
}

async function fetchConfig() {
  loading.value = true;
  error.value = null;
  try {
    const config = await api.get<MockConfig>("/mock/config");
    applyConfig(config);
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load configuration";
  } finally {
    loading.value = false;
  }
}

async function saveEventGeneration() {
  savingEventGeneration.value = true;
  error.value = null;
  try {
    const config = await api.put<MockConfig>("/mock/config", {
      event_generation: {
        auto_deliver: eventGeneration.value.auto_deliver,
        delivery_delay_ms: eventGeneration.value.delivery_delay_ms,
        default_delivery_status_code: eventGeneration.value.default_delivery_status_code,
        auto_fail_rate: eventGeneration.value.auto_fail_rate,
      },
    });
    applyConfig(config);
    showSuccess("Event generation settings saved.");
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to save event generation settings";
  } finally {
    savingEventGeneration.value = false;
  }
}

async function saveDomainBehavior() {
  savingDomainBehavior.value = true;
  error.value = null;
  try {
    const config = await api.put<MockConfig>("/mock/config", {
      domain_behavior: {
        domain_auto_verify: domainBehavior.value.domain_auto_verify,
        sandbox_domain: domainBehavior.value.sandbox_domain,
      },
    });
    applyConfig(config);
    showSuccess("Domain behavior settings saved.");
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to save domain behavior settings";
  } finally {
    savingDomainBehavior.value = false;
  }
}

async function saveWebhookDelivery() {
  savingWebhookDelivery.value = true;
  error.value = null;
  try {
    const config = await api.put<MockConfig>("/mock/config", {
      webhook_delivery: {
        webhook_retry_mode: webhookDelivery.value.webhook_retry_mode,
        webhook_timeout_ms: webhookDelivery.value.webhook_timeout_ms,
      },
    });
    applyConfig(config);
    showSuccess("Webhook delivery settings saved.");
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to save webhook delivery settings";
  } finally {
    savingWebhookDelivery.value = false;
  }
}

async function saveAuthentication() {
  savingAuthentication.value = true;
  error.value = null;
  try {
    const config = await api.put<MockConfig>("/mock/config", {
      authentication: {
        auth_mode: authentication.value.auth_mode,
      },
    });
    applyConfig(config);
    showSuccess("Authentication settings saved.");
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to save authentication settings";
  } finally {
    savingAuthentication.value = false;
  }
}

async function saveStorage() {
  savingStorage.value = true;
  error.value = null;
  try {
    const config = await api.put<MockConfig>("/mock/config", {
      storage: {
        store_attachment_bytes: storage.value.store_attachment_bytes,
        max_messages: storage.value.max_messages,
        max_events: storage.value.max_events,
      },
    });
    applyConfig(config);
    showSuccess("Storage settings saved.");
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to save storage settings";
  } finally {
    savingStorage.value = false;
  }
}

async function fetchDomains() {
  domainsLoading.value = true;
  try {
    const resp = await api.get<DomainsResponse>("/v4/domains");
    domains.value = resp.items || [];
    if (domains.value.length > 0 && !resetDomain.value) {
      resetDomain.value = domains.value[0].name;
    }
  } catch {
    // Silently fail — domains are only needed for per-domain reset
  } finally {
    domainsLoading.value = false;
  }
}

async function resetAll() {
  if (!window.confirm("Reset ALL data? This will clear everything and return to a fresh state.")) return;
  resetting.value = true;
  error.value = null;
  try {
    const resp = await api.post<ResetResponse>("/mock/reset");
    showSuccess(resp.message || "All data has been reset.");
    await fetchConfig();
    await fetchDomains();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to reset all data";
  } finally {
    resetting.value = false;
  }
}

async function resetMessages() {
  if (!window.confirm("Reset all messages and events? Configuration and domain setup will be preserved.")) return;
  resetting.value = true;
  error.value = null;
  try {
    const resp = await api.post<ResetResponse>("/mock/reset/messages");
    showSuccess(resp.message || "Messages and events have been reset.");
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to reset messages and events";
  } finally {
    resetting.value = false;
  }
}

async function resetForDomain() {
  if (!resetDomain.value) return;
  if (!window.confirm(`Reset all data for domain "${resetDomain.value}"?`)) return;
  resetting.value = true;
  error.value = null;
  try {
    const resp = await api.post<ResetResponse>(`/mock/reset/${encodeURIComponent(resetDomain.value)}`);
    showSuccess(resp.message || `Data for domain ${resetDomain.value} has been reset.`);
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to reset domain data";
  } finally {
    resetting.value = false;
  }
}

// --- Lifecycle ---

onMounted(() => {
  fetchConfig();
  fetchDomains();
});
</script>

<template>
  <div class="page">
    <h1>Settings</h1>

    <!-- Global feedback -->
    <div
      v-if="error"
      class="error"
    >
      {{ error }}
    </div>

    <div
      v-if="successMessage"
      class="success"
    >
      {{ successMessage }}
    </div>

    <div
      v-if="loading"
      class="loading-text"
    >
      Loading configuration...
    </div>

    <template v-if="!loading">
      <!-- Event Generation -->
      <div class="card-section">
        <div class="section-header">
          <h2>Event Generation</h2>
        </div>
        <p class="section-description">
          Controls how the mock server automatically generates delivery events for accepted messages.
        </p>

        <div class="settings-grid">
          <div class="setting-row">
            <div class="setting-info">
              <label for="auto-deliver">Auto Deliver</label>
              <span class="setting-help">Automatically generate delivered events after messages are accepted</span>
            </div>
            <div class="setting-control">
              <input
                id="auto-deliver"
                v-model="eventGeneration.auto_deliver"
                type="checkbox"
                class="checkbox-input"
              >
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label for="delivery-delay">Delivery Delay (ms)</label>
              <span class="setting-help">Delay in milliseconds before generating a delivered event</span>
            </div>
            <div class="setting-control">
              <input
                id="delivery-delay"
                v-model.number="eventGeneration.delivery_delay_ms"
                type="number"
                min="0"
                class="filter-input number-input"
              >
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label for="default-status-code">Default Delivery Status Code</label>
              <span class="setting-help">SMTP status code used for delivered events</span>
            </div>
            <div class="setting-control">
              <input
                id="default-status-code"
                v-model.number="eventGeneration.default_delivery_status_code"
                type="number"
                min="200"
                max="599"
                class="filter-input number-input"
              >
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label for="auto-fail-rate">Auto Fail Rate</label>
              <span class="setting-help">Fraction of messages (0.0 - 1.0) that auto-generate failed events</span>
            </div>
            <div class="setting-control">
              <input
                id="auto-fail-rate"
                v-model.number="eventGeneration.auto_fail_rate"
                type="number"
                min="0"
                max="1"
                step="0.01"
                class="filter-input number-input"
              >
            </div>
          </div>
        </div>

        <div class="section-actions">
          <button
            class="btn btn-primary"
            :disabled="savingEventGeneration"
            @click="saveEventGeneration"
          >
            {{ savingEventGeneration ? "Saving..." : "Save Event Generation" }}
          </button>
        </div>
      </div>

      <!-- Domain Behavior -->
      <div class="card-section">
        <div class="section-header">
          <h2>Domain Behavior</h2>
        </div>
        <p class="section-description">
          Controls how domains are created and verified in the mock server.
        </p>

        <div class="settings-grid">
          <div class="setting-row">
            <div class="setting-info">
              <label for="domain-auto-verify">Domain Auto Verify</label>
              <span class="setting-help">New domains are created as active with valid DNS records</span>
            </div>
            <div class="setting-control">
              <input
                id="domain-auto-verify"
                v-model="domainBehavior.domain_auto_verify"
                type="checkbox"
                class="checkbox-input"
              >
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label for="sandbox-domain">Sandbox Domain</label>
              <span class="setting-help">Pre-seeded sandbox domain name</span>
            </div>
            <div class="setting-control">
              <input
                id="sandbox-domain"
                v-model="domainBehavior.sandbox_domain"
                type="text"
                placeholder="sandbox123.mailgun.org"
                class="filter-input text-input"
              >
            </div>
          </div>
        </div>

        <div class="section-actions">
          <button
            class="btn btn-primary"
            :disabled="savingDomainBehavior"
            @click="saveDomainBehavior"
          >
            {{ savingDomainBehavior ? "Saving..." : "Save Domain Behavior" }}
          </button>
        </div>
      </div>

      <!-- Webhook Delivery -->
      <div class="card-section">
        <div class="section-header">
          <h2>Webhook Delivery</h2>
        </div>
        <p class="section-description">
          Controls how the mock server delivers webhook events to configured URLs.
        </p>

        <div class="settings-grid">
          <div class="setting-row">
            <div class="setting-info">
              <label for="webhook-retry-mode">Retry Mode</label>
              <span class="setting-help">How webhook delivery retries are handled</span>
            </div>
            <div class="setting-control">
              <select
                id="webhook-retry-mode"
                v-model="webhookDelivery.webhook_retry_mode"
                class="select-input"
              >
                <option value="immediate">
                  Immediate
                </option>
                <option value="realistic">
                  Realistic
                </option>
              </select>
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label for="webhook-timeout">Timeout (ms)</label>
              <span class="setting-help">Timeout in milliseconds for webhook HTTP requests</span>
            </div>
            <div class="setting-control">
              <input
                id="webhook-timeout"
                v-model.number="webhookDelivery.webhook_timeout_ms"
                type="number"
                min="0"
                class="filter-input number-input"
              >
            </div>
          </div>
        </div>

        <div class="section-actions">
          <button
            class="btn btn-primary"
            :disabled="savingWebhookDelivery"
            @click="saveWebhookDelivery"
          >
            {{ savingWebhookDelivery ? "Saving..." : "Save Webhook Delivery" }}
          </button>
        </div>
      </div>

      <!-- Authentication -->
      <div class="card-section">
        <div class="section-header">
          <h2>Authentication</h2>
        </div>
        <p class="section-description">
          Controls how API authentication is handled by the mock server.
        </p>

        <div class="settings-grid">
          <div class="setting-row">
            <div class="setting-info">
              <label for="auth-mode">Auth Mode</label>
              <span class="setting-help">Whether to accept any credentials or validate them</span>
            </div>
            <div class="setting-control">
              <select
                id="auth-mode"
                v-model="authentication.auth_mode"
                class="select-input"
              >
                <option value="accept_any">
                  Accept Any
                </option>
                <option value="validate">
                  Validate
                </option>
              </select>
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label>Signing Key</label>
              <span class="setting-help">Webhook signing key (read-only)</span>
            </div>
            <div class="setting-control">
              <span class="mono readonly-value">{{ authentication.signing_key }}</span>
            </div>
          </div>
        </div>

        <div class="section-actions">
          <button
            class="btn btn-primary"
            :disabled="savingAuthentication"
            @click="saveAuthentication"
          >
            {{ savingAuthentication ? "Saving..." : "Save Authentication" }}
          </button>
        </div>
      </div>

      <!-- Storage -->
      <div class="card-section">
        <div class="section-header">
          <h2>Storage</h2>
        </div>
        <p class="section-description">
          Controls how messages and events are stored and retained.
        </p>

        <div class="settings-grid">
          <div class="setting-row">
            <div class="setting-info">
              <label for="store-attachments">Store Attachment Bytes</label>
              <span class="setting-help">Whether to store actual attachment content</span>
            </div>
            <div class="setting-control">
              <input
                id="store-attachments"
                v-model="storage.store_attachment_bytes"
                type="checkbox"
                class="checkbox-input"
              >
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label for="max-messages">Max Messages</label>
              <span class="setting-help">Maximum messages to retain (0 = unlimited)</span>
            </div>
            <div class="setting-control">
              <input
                id="max-messages"
                v-model.number="storage.max_messages"
                type="number"
                min="0"
                class="filter-input number-input"
              >
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label for="max-events">Max Events</label>
              <span class="setting-help">Maximum events to retain (0 = unlimited)</span>
            </div>
            <div class="setting-control">
              <input
                id="max-events"
                v-model.number="storage.max_events"
                type="number"
                min="0"
                class="filter-input number-input"
              >
            </div>
          </div>
        </div>

        <div class="section-actions">
          <button
            class="btn btn-primary"
            :disabled="savingStorage"
            @click="saveStorage"
          >
            {{ savingStorage ? "Saving..." : "Save Storage" }}
          </button>
        </div>
      </div>

      <!-- Data Reset -->
      <div class="card-section card-danger">
        <div class="section-header">
          <h2>Data Reset</h2>
        </div>
        <p class="section-description">
          Reset stored data. These actions cannot be undone.
        </p>

        <div class="reset-actions">
          <div class="reset-row">
            <div class="reset-info">
              <strong>Reset All Data</strong>
              <span class="setting-help">Clear everything and return to a fresh state, including configuration.</span>
            </div>
            <button
              class="btn btn-danger"
              :disabled="resetting"
              @click="resetAll"
            >
              {{ resetting ? "Resetting..." : "Reset All Data" }}
            </button>
          </div>

          <div class="reset-row">
            <div class="reset-info">
              <strong>Reset Messages &amp; Events</strong>
              <span class="setting-help">Clear all messages and events. Configuration and domain setup are preserved.</span>
            </div>
            <button
              class="btn btn-danger"
              :disabled="resetting"
              @click="resetMessages"
            >
              {{ resetting ? "Resetting..." : "Reset Messages & Events" }}
            </button>
          </div>

          <div class="reset-row">
            <div class="reset-info">
              <strong>Reset Per Domain</strong>
              <span class="setting-help">Clear all data for a specific domain.</span>
            </div>
            <div class="reset-domain-controls">
              <select
                v-model="resetDomain"
                class="select-input"
                :disabled="domainsLoading || domains.length === 0"
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
              <button
                class="btn btn-danger"
                :disabled="resetting || !resetDomain"
                @click="resetForDomain"
              >
                {{ resetting ? "Resetting..." : "Reset Domain" }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.loading-text {
  text-align: center;
  color: var(--color-text-secondary, #64748b);
  padding: 2rem;
}

.error {
  padding: 0.75rem 1rem;
  margin-bottom: 1rem;
  background: var(--color-badge-danger-bg, #fee2e2);
  color: var(--color-badge-danger-text, #991b1b);
  border-radius: 0.375rem;
}

.success {
  padding: 0.75rem 1rem;
  margin-bottom: 1rem;
  background: var(--color-badge-success-bg, #dcfce7);
  color: var(--color-badge-success-text, #166534);
  border-radius: 0.375rem;
}

.card-section {
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.5rem;
  padding: 1.5rem;
  margin-bottom: 1.5rem;
}

.card-danger {
  border-color: var(--color-badge-danger-text, #991b1b);
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.section-header h2 {
  margin: 0;
  font-size: 1.125rem;
}

.section-description {
  margin: 0 0 1rem;
  font-size: 0.875rem;
  color: var(--color-text-secondary, #64748b);
}

.settings-grid {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.setting-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem 0;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
}

.setting-row:last-child {
  border-bottom: none;
}

.setting-info {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  flex: 1;
  min-width: 0;
}

.setting-info label {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--color-text-primary, #1e293b);
}

.setting-help {
  font-size: 0.75rem;
  color: var(--color-text-secondary, #64748b);
}

.setting-control {
  flex-shrink: 0;
  margin-left: 1rem;
}

.filter-input {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  background: var(--color-bg-primary, #ffffff);
  color: var(--color-text-primary, #1e293b);
}

.filter-input:focus {
  outline: 2px solid var(--color-primary, #3b82f6);
  outline-offset: -1px;
}

.number-input {
  width: 8rem;
  text-align: right;
}

.text-input {
  width: 16rem;
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

.checkbox-input {
  width: 1.125rem;
  height: 1.125rem;
  cursor: pointer;
  accent-color: var(--color-primary, #3b82f6);
}

.readonly-value {
  font-size: 0.8125rem;
  color: var(--color-text-secondary, #64748b);
  background: var(--color-bg-subtle, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  padding: 0.375rem 0.75rem;
  display: inline-block;
  word-break: break-all;
}

.mono {
  font-family: monospace;
}

.section-actions {
  margin-top: 1rem;
  display: flex;
  justify-content: flex-end;
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

.btn-danger {
  background: var(--color-badge-danger-bg, #fee2e2);
  color: var(--color-badge-danger-text, #991b1b);
  border-color: var(--color-badge-danger-text, #991b1b);
}

.btn-danger:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-danger:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Reset section */
.reset-actions {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.reset-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem 0;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
}

.reset-row:last-child {
  border-bottom: none;
}

.reset-info {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  flex: 1;
  min-width: 0;
}

.reset-info strong {
  font-size: 0.875rem;
  color: var(--color-text-primary, #1e293b);
}

.reset-domain-controls {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-shrink: 0;
  margin-left: 1rem;
}
</style>
