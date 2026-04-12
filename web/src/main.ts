import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import { fetchAuthConfig } from "@/auth/config";
import { initMsal, getActiveAccount, signIn } from "@/auth/msalInstance";
import { useAuth } from "@/composables/useAuth";
import { startWebSocket } from "@/composables/useWebSocket";

async function bootstrap() {
  const cfg = await fetchAuthConfig();

  if (!cfg.enabled) {
    startWebSocket();
    createApp(App).use(router).mount("#app");
    return;
  }

  const msal = await initMsal(cfg);
  if (!msal) throw new Error("MSAL initialization failed");
  await msal.handleRedirectPromise();

  const account = getActiveAccount();
  if (!account) {
    signIn();
    return; // redirect away, won't reach mount
  }

  // Auth ready — sync reactive state, start WS, mount
  const { initAuthState } = useAuth();
  initAuthState();
  startWebSocket();
  createApp(App).use(router).mount("#app");
}

bootstrap().catch((err) => {
  console.error("Failed to bootstrap app:", err);
  document.body.innerText = "Failed to load app: " + err.message;
});
