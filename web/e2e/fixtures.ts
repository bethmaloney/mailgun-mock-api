import { test as base } from "@playwright/test";

const API_BASE = "http://localhost:8026";
const API_KEY = "test-api-key";
const AUTH_HEADER = "Basic " + Buffer.from(`api:${API_KEY}`).toString("base64");

interface SendMessageOpts {
  from: string;
  to: string;
  subject: string;
  text?: string;
  html?: string;
}

interface CreateWebhookOpts {
  id: string;
  url: string;
}

interface CreateRouteOpts {
  priority?: number;
  expression: string;
  action: string[];
  description?: string;
}

export class ApiHelper {
  private async request(
    method: string,
    path: string,
    body?: Record<string, unknown>,
  ): Promise<Response> {
    const res = await fetch(`${API_BASE}${path}`, {
      method,
      headers: {
        Authorization: AUTH_HEADER,
        ...(body ? { "Content-Type": "application/json" } : {}),
      },
      body: body ? JSON.stringify(body) : undefined,
    });
    return res;
  }

  private async formRequest(
    method: string,
    path: string,
    fields: Record<string, string>,
  ): Promise<Response> {
    const form = new URLSearchParams();
    for (const [k, v] of Object.entries(fields)) {
      form.append(k, v);
    }
    return fetch(`${API_BASE}${path}`, {
      method,
      headers: {
        Authorization: AUTH_HEADER,
        "Content-Type": "application/x-www-form-urlencoded",
      },
      body: form.toString(),
    });
  }

  /** Clear all data and reset config to defaults. */
  async reset(): Promise<void> {
    const res = await fetch(`${API_BASE}/mock/reset`, { method: "POST" });
    if (!res.ok) throw new Error(`Reset failed: ${res.status}`);
  }

  /** Create a domain via the Mailgun API. */
  async createDomain(name: string): Promise<Record<string, unknown>> {
    const res = await this.formRequest("POST", "/v4/domains", {
      name,
    });
    if (!res.ok) throw new Error(`createDomain failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Send a message via the Mailgun API. Returns the response with message id. */
  async sendMessage(
    domain: string,
    opts: SendMessageOpts,
  ): Promise<Record<string, unknown>> {
    const fields: Record<string, string> = {
      from: opts.from,
      to: opts.to,
      subject: opts.subject,
    };
    if (opts.text) fields.text = opts.text;
    if (opts.html) fields.html = opts.html;

    const res = await this.formRequest(
      "POST",
      `/v3/${domain}/messages`,
      fields,
    );
    if (!res.ok) throw new Error(`sendMessage failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Create a domain webhook. */
  async createWebhook(
    domain: string,
    opts: CreateWebhookOpts,
  ): Promise<Record<string, unknown>> {
    const res = await this.formRequest(
      "POST",
      `/v3/domains/${domain}/webhooks`,
      { id: opts.id, url: opts.url },
    );
    if (!res.ok) throw new Error(`createWebhook failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Create a route. */
  async createRoute(opts: CreateRouteOpts): Promise<Record<string, unknown>> {
    const fields: Record<string, string> = {
      expression: opts.expression,
    };
    if (opts.priority !== undefined) fields.priority = String(opts.priority);
    if (opts.description) fields.description = opts.description;
    // Mailgun API accepts multiple action[] fields; use form encoding
    const form = new URLSearchParams();
    form.append("expression", opts.expression);
    if (opts.priority !== undefined) form.append("priority", String(opts.priority));
    if (opts.description) form.append("description", opts.description);
    for (const a of opts.action) {
      form.append("action", a);
    }
    const res = await fetch(`${API_BASE}/v3/routes`, {
      method: "POST",
      headers: {
        Authorization: AUTH_HEADER,
        "Content-Type": "application/x-www-form-urlencoded",
      },
      body: form.toString(),
    });
    if (!res.ok) throw new Error(`createRoute failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Create a mailing list. */
  async createMailingList(
    address: string,
    name?: string,
    description?: string,
  ): Promise<Record<string, unknown>> {
    const fields: Record<string, string> = { address };
    if (name) fields.name = name;
    if (description) fields.description = description;
    const res = await this.formRequest("POST", "/v3/lists", fields);
    if (!res.ok) throw new Error(`createMailingList failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Add a member to a mailing list. */
  async addMemberToList(
    listAddress: string,
    memberAddress: string,
    name?: string,
  ): Promise<Record<string, unknown>> {
    const fields: Record<string, string> = { address: memberAddress, subscribed: "true" };
    if (name) fields.name = name;
    const res = await this.formRequest(
      "POST",
      `/v3/lists/${encodeURIComponent(listAddress)}/members`,
      fields,
    );
    if (!res.ok) throw new Error(`addMemberToList failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Trigger a mock event for a message. */
  async triggerEvent(
    domain: string,
    action: string,
    storageKey: string,
    body?: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    const res = await this.request(
      "POST",
      `/mock/events/${domain}/${action}/${storageKey}`,
      body,
    );
    if (!res.ok) throw new Error(`triggerEvent failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Trigger a webhook delivery via the mock API. */
  async triggerWebhook(opts: {
    domain: string;
    event_type: string;
    recipient?: string;
    message_id?: string;
  }): Promise<Record<string, unknown>> {
    const res = await this.request("POST", "/mock/webhooks/trigger", opts);
    if (!res.ok) throw new Error(`triggerWebhook failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Send a message with extra fields (tags, custom headers, custom variables). */
  async sendMessageWithExtras(
    domain: string,
    opts: {
      from: string;
      to: string;
      subject: string;
      text?: string;
      html?: string;
      tags?: string[];
      headers?: Record<string, string>;
      variables?: Record<string, string>;
    },
  ): Promise<Record<string, unknown>> {
    const form = new URLSearchParams();
    form.append("from", opts.from);
    form.append("to", opts.to);
    form.append("subject", opts.subject);
    if (opts.text) form.append("text", opts.text);
    if (opts.html) form.append("html", opts.html);
    if (opts.tags) {
      for (const tag of opts.tags) {
        form.append("o:tag", tag);
      }
    }
    if (opts.headers) {
      for (const [k, v] of Object.entries(opts.headers)) {
        form.append(`h:${k}`, v);
      }
    }
    if (opts.variables) {
      for (const [k, v] of Object.entries(opts.variables)) {
        form.append(`v:${k}`, v);
      }
    }
    const res = await fetch(`${API_BASE}/v3/${domain}/messages`, {
      method: "POST",
      headers: {
        Authorization: AUTH_HEADER,
        "Content-Type": "application/x-www-form-urlencoded",
      },
      body: form.toString(),
    });
    if (!res.ok) throw new Error(`sendMessageWithExtras failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Send a message with a file attachment via multipart form. */
  async sendMessageWithAttachment(
    domain: string,
    opts: {
      from: string;
      to: string;
      subject: string;
      text?: string;
      filename: string;
      contentType: string;
      content: Buffer;
    },
  ): Promise<Record<string, unknown>> {
    const formData = new FormData();
    formData.append("from", opts.from);
    formData.append("to", opts.to);
    formData.append("subject", opts.subject);
    if (opts.text) formData.append("text", opts.text);
    formData.append("attachment", new Blob([opts.content], { type: opts.contentType }), opts.filename);
    const res = await fetch(`${API_BASE}/v3/${domain}/messages`, {
      method: "POST",
      headers: {
        Authorization: AUTH_HEADER,
      },
      body: formData,
    });
    if (!res.ok) throw new Error(`sendMessageWithAttachment failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Update mock configuration. */
  async updateConfig(
    config: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    const res = await this.request("PUT", "/mock/config", config);
    if (!res.ok) throw new Error(`updateConfig failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Create a template via the Mailgun API. */
  async createTemplate(
    domain: string,
    opts: {
      name: string;
      description?: string;
      template?: string;
      tag?: string;
      engine?: string;
      comment?: string;
    },
  ): Promise<Record<string, unknown>> {
    const fields: Record<string, string> = { name: opts.name };
    if (opts.description) fields.description = opts.description;
    if (opts.template) fields.template = opts.template;
    if (opts.tag) fields.tag = opts.tag;
    if (opts.engine) fields.engine = opts.engine;
    if (opts.comment) fields.comment = opts.comment;
    const res = await this.formRequest("POST", `/v3/${domain}/templates`, fields);
    if (!res.ok) throw new Error(`createTemplate failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Add a bounce suppression entry. */
  async addBounce(
    domain: string,
    address: string,
    code?: string,
    error?: string,
  ): Promise<Record<string, unknown>> {
    const fields: Record<string, string> = { address };
    if (code) fields.code = code;
    if (error) fields.error = error;
    const res = await this.formRequest("POST", `/v3/${domain}/bounces`, fields);
    if (!res.ok) throw new Error(`addBounce failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Add a complaint suppression entry. */
  async addComplaint(
    domain: string,
    address: string,
  ): Promise<Record<string, unknown>> {
    const res = await this.formRequest("POST", `/v3/${domain}/complaints`, { address });
    if (!res.ok) throw new Error(`addComplaint failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Add an unsubscribe suppression entry. */
  async addUnsubscribe(
    domain: string,
    address: string,
    tag?: string,
  ): Promise<Record<string, unknown>> {
    const fields: Record<string, string> = { address };
    if (tag) fields.tag = tag;
    const res = await this.formRequest("POST", `/v3/${domain}/unsubscribes`, fields);
    if (!res.ok) throw new Error(`addUnsubscribe failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Add an allowlist entry (address or domain). */
  async addAllowlistEntry(
    domain: string,
    value: string,
    type: "address" | "domain",
  ): Promise<Record<string, unknown>> {
    const fields: Record<string, string> = type === "domain" ? { domain: value } : { address: value };
    const res = await this.formRequest("POST", `/v3/${domain}/whitelists`, fields);
    if (!res.ok) throw new Error(`addAllowlistEntry failed: ${res.status} ${await res.text()}`);
    return res.json();
  }

  /** Create a template version via the Mailgun API. */
  async createTemplateVersion(
    domain: string,
    templateName: string,
    opts: {
      template: string;
      tag: string;
      engine?: string;
      comment?: string;
      active?: string;
    },
  ): Promise<Record<string, unknown>> {
    const fields: Record<string, string> = {
      template: opts.template,
      tag: opts.tag,
    };
    if (opts.engine) fields.engine = opts.engine;
    if (opts.comment) fields.comment = opts.comment;
    if (opts.active) fields.active = opts.active;
    const res = await this.formRequest(
      "POST",
      `/v3/${domain}/templates/${templateName}/versions`,
      fields,
    );
    if (!res.ok) throw new Error(`createTemplateVersion failed: ${res.status} ${await res.text()}`);
    return res.json();
  }
}

export const test = base.extend<{ api: ApiHelper }>({
  // eslint-disable-next-line no-empty-pattern
  api: async ({}, use) => {
    const api = new ApiHelper();
    await api.reset();
    await use(api);
  },
});

export { expect } from "@playwright/test";
