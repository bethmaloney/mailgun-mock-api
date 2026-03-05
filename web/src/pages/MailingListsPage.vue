<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { api } from "@/api/client";
import DataTable from "@/components/DataTable.vue";
import Pagination from "@/components/Pagination.vue";
import type { Column } from "@/components/DataTable.vue";

interface MailingList {
  address: string;
  name: string;
  description: string;
  access_level: string;
  reply_preference: string;
  members_count: number;
  created_at: string;
}

interface Paging {
  next?: string;
  previous?: string;
}

interface ListsResponse {
  items: MailingList[];
  paging: Paging;
}

interface ListDetailResponse {
  list: MailingList;
}

interface Member {
  address: string;
  name: string;
  subscribed: boolean;
  vars: Record<string, unknown>;
}

interface MembersResponse {
  items: Member[];
  paging: Paging;
}

// Lists state
const lists = ref<MailingList[]>([]);
const listsPaging = ref<Paging>({});
const loading = ref(true);
const error = ref<string | null>(null);

// Create list form
const newListAddress = ref("");
const newListName = ref("");
const newListDescription = ref("");

// Detail view
const selectedList = ref<MailingList | null>(null);
const detailLoading = ref(false);

// Members state
const members = ref<Member[]>([]);
const membersPaging = ref<Paging>({});
const membersLoading = ref(false);

// Add member form
const newMemberAddress = ref("");
const newMemberName = ref("");

const listColumns: Column[] = [
  { key: "address", label: "Address" },
  { key: "name", label: "Name" },
  { key: "members_count", label: "Members" },
  { key: "access_level", label: "Access Level" },
  { key: "created_at", label: "Created At" },
];

const memberColumns: Column[] = [
  { key: "address", label: "Address" },
  { key: "name", label: "Name" },
  { key: "subscribed", label: "Subscribed" },
  { key: "vars", label: "Vars" },
  { key: "actions", label: "" },
];

function formatDate(dateStr: string): string {
  if (!dateStr) return "-";
  try {
    const date = new Date(dateStr);
    return date.toLocaleString();
  } catch {
    return dateStr;
  }
}

const listTableRows = computed(() =>
  lists.value.map((list) => ({
    ...list,
    created_at: formatDate(list.created_at),
    _raw: list,
  }))
);

const memberTableRows = computed(() =>
  members.value.map((member) => ({
    ...member,
    subscribed: member.subscribed ? "Yes" : "No",
    vars: Object.keys(member.vars || {}).length > 0
      ? JSON.stringify(member.vars)
      : "-",
    _raw: member,
  }))
);

const listsHasNext = computed(() => !!listsPaging.value.next);
const listsHasPrevious = computed(() => !!listsPaging.value.previous);
const membersHasNext = computed(() => !!membersPaging.value.next);
const membersHasPrevious = computed(() => !!membersPaging.value.previous);

function buildListsUrl(pagingUrl?: string): string {
  if (pagingUrl) {
    try {
      const u = new URL(pagingUrl);
      return u.pathname + u.search;
    } catch {
      return pagingUrl;
    }
  }
  return "/v3/lists/pages";
}

function buildMembersUrl(listAddress: string, pagingUrl?: string): string {
  if (pagingUrl) {
    try {
      const u = new URL(pagingUrl);
      return u.pathname + u.search;
    } catch {
      return pagingUrl;
    }
  }
  return `/v3/lists/${encodeURIComponent(listAddress)}/members/pages`;
}

async function fetchLists(pagingUrl?: string) {
  loading.value = true;
  error.value = null;
  try {
    const url = buildListsUrl(pagingUrl);
    const resp = await api.get<ListsResponse>(url);
    lists.value = resp.items || [];
    listsPaging.value = resp.paging || {};
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load mailing lists";
  } finally {
    loading.value = false;
  }
}

function listsLoadNext() {
  if (listsPaging.value.next) fetchLists(listsPaging.value.next);
}

function listsLoadPrev() {
  if (listsPaging.value.previous) fetchLists(listsPaging.value.previous);
}

async function createList() {
  if (!newListAddress.value) return;
  error.value = null;
  try {
    const fields: Record<string, string> = {
      address: newListAddress.value,
      access_level: "readonly",
    };
    if (newListName.value) fields.name = newListName.value;
    if (newListDescription.value) fields.description = newListDescription.value;
    await api.postForm("/v3/lists", fields);
    newListAddress.value = "";
    newListName.value = "";
    newListDescription.value = "";
    await fetchLists();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to create mailing list";
  }
}

async function deleteList(address: string) {
  if (!window.confirm(`Delete mailing list ${address}?`)) return;
  error.value = null;
  try {
    await api.del(`/v3/lists/${encodeURIComponent(address)}`);
    if (selectedList.value && selectedList.value.address === address) {
      selectedList.value = null;
      members.value = [];
    }
    await fetchLists();
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to delete mailing list";
  }
}

async function viewListDetail(row: Record<string, unknown>) {
  const raw = row._raw as MailingList;
  if (selectedList.value && selectedList.value.address === raw.address) {
    selectedList.value = null;
    members.value = [];
    return;
  }
  detailLoading.value = true;
  error.value = null;
  try {
    const resp = await api.get<ListDetailResponse>(
      `/v3/lists/${encodeURIComponent(raw.address)}`
    );
    selectedList.value = resp.list;
    await fetchMembers(raw.address);
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load list detail";
  } finally {
    detailLoading.value = false;
  }
}

async function fetchMembers(listAddress: string, pagingUrl?: string) {
  membersLoading.value = true;
  try {
    const url = buildMembersUrl(listAddress, pagingUrl);
    const resp = await api.get<MembersResponse>(url);
    members.value = resp.items || [];
    membersPaging.value = resp.paging || {};
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to load members";
  } finally {
    membersLoading.value = false;
  }
}

function membersLoadNext() {
  if (selectedList.value && membersPaging.value.next) {
    fetchMembers(selectedList.value.address, membersPaging.value.next);
  }
}

function membersLoadPrev() {
  if (selectedList.value && membersPaging.value.previous) {
    fetchMembers(selectedList.value.address, membersPaging.value.previous);
  }
}

async function addMember() {
  if (!selectedList.value || !newMemberAddress.value) return;
  error.value = null;
  try {
    const fields: Record<string, string> = {
      address: newMemberAddress.value,
      subscribed: "true",
    };
    if (newMemberName.value) fields.name = newMemberName.value;
    await api.postForm(
      `/v3/lists/${encodeURIComponent(selectedList.value.address)}/members`,
      fields,
    );
    newMemberAddress.value = "";
    newMemberName.value = "";
    await fetchMembers(selectedList.value.address);
    // Refresh the list to update member count
    await fetchLists();
    // Re-fetch detail to get updated members_count
    const resp = await api.get<ListDetailResponse>(
      `/v3/lists/${encodeURIComponent(selectedList.value.address)}`
    );
    selectedList.value = resp.list;
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to add member";
  }
}

async function deleteMember(memberAddress: string) {
  if (!selectedList.value) return;
  if (!window.confirm(`Remove ${memberAddress} from this list?`)) return;
  error.value = null;
  try {
    await api.del(
      `/v3/lists/${encodeURIComponent(selectedList.value.address)}/members/${encodeURIComponent(memberAddress)}`
    );
    await fetchMembers(selectedList.value.address);
    // Refresh the list to update member count
    await fetchLists();
    // Re-fetch detail to get updated members_count
    const resp = await api.get<ListDetailResponse>(
      `/v3/lists/${encodeURIComponent(selectedList.value.address)}`
    );
    selectedList.value = resp.list;
  } catch (e: unknown) {
    const err = e as { message?: string };
    error.value = err.message || "Failed to remove member";
  }
}

onMounted(() => fetchLists());
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h1>Mailing Lists</h1>
    </div>

    <!-- Create List Form -->
    <div class="create-form">
      <input
        v-model="newListAddress"
        type="text"
        placeholder="List address (e.g. devs@lists.example.com)"
        class="filter-input input-wide"
        @keyup.enter="createList"
      >
      <input
        v-model="newListName"
        type="text"
        placeholder="Name"
        class="filter-input"
        @keyup.enter="createList"
      >
      <input
        v-model="newListDescription"
        type="text"
        placeholder="Description"
        class="filter-input"
        @keyup.enter="createList"
      >
      <button
        class="btn btn-primary"
        :disabled="!newListAddress"
        @click="createList"
      >
        Create List
      </button>
    </div>

    <div
      v-if="error"
      class="error"
    >
      {{ error }}
    </div>

    <DataTable
      :columns="listColumns"
      :rows="listTableRows"
      :loading="loading"
    >
      <template #cell-address="{ row }">
        <a
          href="#"
          class="list-link"
          @click.prevent="viewListDetail(row)"
        >
          {{ row.address }}
        </a>
      </template>
      <template #cell-created_at="{ row }">
        <span class="date-cell">
          {{ row.created_at }}
          <button
            class="btn-icon btn-delete"
            title="Delete list"
            @click.stop="deleteList(String((row._raw as MailingList).address))"
          >
            x
          </button>
        </span>
      </template>
    </DataTable>

    <Pagination
      :has-next="listsHasNext"
      :has-previous="listsHasPrevious"
      @next="listsLoadNext"
      @previous="listsLoadPrev"
    />

    <!-- Detail Panel -->
    <div
      v-if="detailLoading"
      class="detail-panel loading"
    >
      Loading list detail...
    </div>
    <div
      v-else-if="selectedList"
      class="detail-panel"
    >
      <div class="detail-header">
        <h2>List Detail</h2>
        <button
          class="btn btn-sm"
          @click="selectedList = null; members = []"
        >
          Close
        </button>
      </div>

      <div class="detail-grid">
        <div class="detail-field">
          <label>Address</label>
          <span class="mono">{{ selectedList.address }}</span>
        </div>
        <div class="detail-field">
          <label>Name</label>
          <span>{{ selectedList.name || "-" }}</span>
        </div>
        <div class="detail-field">
          <label>Description</label>
          <span>{{ selectedList.description || "-" }}</span>
        </div>
        <div class="detail-field">
          <label>Access Level</label>
          <span>{{ selectedList.access_level }}</span>
        </div>
        <div class="detail-field">
          <label>Reply Preference</label>
          <span>{{ selectedList.reply_preference || "-" }}</span>
        </div>
        <div class="detail-field">
          <label>Members Count</label>
          <span>{{ selectedList.members_count }}</span>
        </div>
      </div>

      <!-- Members Section -->
      <div class="detail-section">
        <h3>Members</h3>

        <!-- Add Member Form -->
        <div class="add-member-form">
          <input
            v-model="newMemberAddress"
            type="text"
            placeholder="Member email address"
            class="filter-input"
            @keyup.enter="addMember"
          >
          <input
            v-model="newMemberName"
            type="text"
            placeholder="Name"
            class="filter-input"
            @keyup.enter="addMember"
          >
          <button
            class="btn btn-primary"
            :disabled="!newMemberAddress"
            @click="addMember"
          >
            Add Member
          </button>
        </div>

        <DataTable
          :columns="memberColumns"
          :rows="memberTableRows"
          :loading="membersLoading"
        >
          <template #cell-actions="{ row }">
            <button
              class="btn-icon btn-delete"
              title="Remove member"
              @click.stop="deleteMember(String((row._raw as Member).address))"
            >
              x
            </button>
          </template>
        </DataTable>

        <Pagination
          :has-next="membersHasNext"
          :has-previous="membersHasPrevious"
          @next="membersLoadNext"
          @previous="membersLoadPrev"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.page-header h1 {
  margin: 0;
}

.create-form {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.filter-input {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.375rem;
  background: var(--color-bg-primary, #ffffff);
  color: var(--color-text-primary, #1e293b);
  min-width: 8rem;
}

.filter-input:focus {
  outline: 2px solid var(--color-primary, #3b82f6);
  outline-offset: -1px;
}

.input-wide {
  min-width: 16rem;
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

.btn-danger:hover {
  opacity: 0.9;
}

.btn-sm {
  padding: 0.25rem 0.5rem;
  font-size: 0.75rem;
  background: var(--color-bg-primary, #ffffff);
}

.btn-sm:hover {
  background: var(--color-bg-hover, #f1f5f9);
}

.btn-icon {
  border: none;
  background: none;
  cursor: pointer;
  padding: 0.125rem 0.375rem;
  border-radius: 0.25rem;
  font-size: 0.75rem;
  line-height: 1;
}

.btn-delete {
  color: var(--color-badge-danger-text, #991b1b);
}

.btn-delete:hover {
  background: var(--color-badge-danger-bg, #fee2e2);
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

.list-link {
  color: var(--color-primary, #3b82f6);
  text-decoration: none;
}

.list-link:hover {
  text-decoration: underline;
}

.date-cell {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.detail-panel {
  margin-top: 1rem;
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 0.5rem;
  padding: 1.5rem;
}

.detail-panel.loading {
  text-align: center;
  color: var(--color-text-secondary, #64748b);
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.detail-header h2 {
  margin: 0;
  font-size: 1.125rem;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
  gap: 0.75rem;
  margin-bottom: 1rem;
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

.mono {
  font-family: monospace;
  font-size: 0.8125rem !important;
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
  margin: 0 0 0.5rem;
}

.add-member-form {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}
</style>
