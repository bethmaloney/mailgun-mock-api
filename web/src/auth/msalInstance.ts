import { PublicClientApplication, type AccountInfo, InteractionRequiredAuthError } from "@azure/msal-browser";
import type { AuthConfig } from "./config";

let msalInstance: PublicClientApplication | null = null;
let msalScopes: string[] = [];

export async function initMsal(config: AuthConfig): Promise<PublicClientApplication | null> {
  if (!config.enabled) {
    return null;
  }

  const instance = new PublicClientApplication({
    auth: {
      clientId: config.clientId,
      authority: `https://login.microsoftonline.com/${config.tenantId}`,
      redirectUri: config.redirectUri,
    },
    cache: {
      cacheLocation: "localStorage",
    },
  });

  await instance.initialize();

  msalInstance = instance;
  msalScopes = config.scopes;

  return instance;
}

export { msalInstance };

export function getActiveAccount(): AccountInfo | null {
  if (!msalInstance) return null;

  const active = msalInstance.getActiveAccount();
  if (active) return active;

  const accounts = msalInstance.getAllAccounts();
  if (accounts.length > 0) {
    msalInstance.setActiveAccount(accounts[0]);
    return accounts[0];
  }

  return null;
}

export async function getAccessToken(): Promise<string | null> {
  if (!msalInstance) return null;

  const account = getActiveAccount();
  if (!account) return null;

  try {
    const result = await msalInstance.acquireTokenSilent({
      scopes: msalScopes,
      account,
    });
    return result.accessToken;
  } catch (err) {
    if (err instanceof InteractionRequiredAuthError) {
      await msalInstance.acquireTokenRedirect({ scopes: msalScopes });
      return null;
    }
    throw err;
  }
}

export function signIn(): void {
  msalInstance?.loginRedirect({ scopes: msalScopes });
}

export function signOut(): void {
  msalInstance?.logoutRedirect({
    postLogoutRedirectUri: window.location.origin,
    account: getActiveAccount(),
  });
}
