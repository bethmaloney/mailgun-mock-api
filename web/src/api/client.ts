/**
 * Simple fetch-based API client for the Mailgun Mock API backend.
 * Base URL is relative so the Vite dev proxy (/mock -> localhost:8025)
 * handles forwarding during development.
 */

import { getAccessToken } from "@/auth/msalInstance";

export interface ApiError {
  message: string;
  status: number;
}

class ApiClient {
  private async request<T>(url: string, options: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = {
      Accept: "application/json",
    };

    // Only set Content-Type for requests with a body
    if (options.body) {
      headers["Content-Type"] = "application/json";
    }

    // Spread options.headers AFTER defaults so callers can override Content-Type
    Object.assign(headers, options.headers as Record<string, string>);

    const token = await getAccessToken();
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }

    const response = await fetch(url, {
      ...options,
      headers,
    });

    if (!response.ok) {
      let message = `Request failed with status ${response.status}`;
      try {
        const errorData = await response.json();
        if (errorData.message) {
          message = errorData.message;
        }
      } catch {
        // Ignore JSON parse errors for error responses
      }
      const error: ApiError = { message, status: response.status };
      throw error;
    }

    // Handle 204 No Content
    if (response.status === 204) {
      return undefined as unknown as T;
    }

    return response.json();
  }

  async get<T>(url: string): Promise<T> {
    return this.request<T>(url, { method: "GET" });
  }

  async post<T>(url: string, data?: unknown): Promise<T> {
    return this.request<T>(url, {
      method: "POST",
      body: data ? JSON.stringify(data) : undefined,
    });
  }

  async put<T>(url: string, data?: unknown): Promise<T> {
    return this.request<T>(url, {
      method: "PUT",
      body: data ? JSON.stringify(data) : undefined,
    });
  }

  async postForm<T>(url: string, data: Record<string, string>): Promise<T> {
    const form = new URLSearchParams();
    for (const [k, v] of Object.entries(data)) {
      form.append(k, v);
    }
    return this.request<T>(url, {
      method: "POST",
      body: form.toString(),
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
    });
  }

  async postFormMulti<T>(url: string, data: URLSearchParams): Promise<T> {
    return this.request<T>(url, {
      method: "POST",
      body: data.toString(),
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
    });
  }

  async del<T>(url: string): Promise<T> {
    return this.request<T>(url, { method: "DELETE" });
  }
}

export const api = new ApiClient();
