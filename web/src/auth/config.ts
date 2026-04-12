export type AuthConfig =
  | { enabled: false }
  | {
      enabled: true;
      tenantId: string;
      clientId: string;
      scopes: string[];
      redirectUri: string;
    };

export async function fetchAuthConfig(): Promise<AuthConfig> {
  const res = await fetch("/mock/auth-config", { headers: { Accept: "application/json" } });
  if (!res.ok) throw new Error(`auth-config fetch failed: ${res.status}`);
  return res.json();
}
