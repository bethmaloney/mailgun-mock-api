import { ref, computed } from "vue";
import { msalInstance, getActiveAccount, signIn, signOut } from "@/auth/msalInstance";
import { EventType } from "@azure/msal-browser";

const user = ref<{ name: string; email: string; oid: string } | null>(null);

function syncUser() {
  const account = getActiveAccount();
  if (account) {
    user.value = {
      name: account.name ?? "",
      email: account.username ?? "",
      oid: account.localAccountId,
    };
  } else {
    user.value = null;
  }
}

function initAuthState() {
  syncUser();
  if (msalInstance !== null) {
    msalInstance.addEventCallback((event) => {
      if (event.eventType === EventType.LOGIN_SUCCESS || event.eventType === EventType.LOGOUT_SUCCESS) {
        syncUser();
      }
    });
  }
}

export function useAuth() {
  return {
    user: computed(() => user.value),
    isAuthenticated: computed(() => user.value !== null),
    signIn,
    signOut,
    initAuthState,
  };
}
