import { createRouter, createWebHistory } from "vue-router";
import type { RouteRecordRaw } from "vue-router";

import DashboardPage from "@/pages/DashboardPage.vue";
import MessagesPage from "@/pages/MessagesPage.vue";
import EventsPage from "@/pages/EventsPage.vue";
import DomainsPage from "@/pages/DomainsPage.vue";
import TemplatesPage from "@/pages/TemplatesPage.vue";
import MailingListsPage from "@/pages/MailingListsPage.vue";
import RoutesPage from "@/pages/RoutesPage.vue";
import SuppressionsPage from "@/pages/SuppressionsPage.vue";
import WebhooksPage from "@/pages/WebhooksPage.vue";
import SettingsPage from "@/pages/SettingsPage.vue";
import TriggerEventsPage from "@/pages/TriggerEventsPage.vue";
import SimulateInboundPage from "@/pages/SimulateInboundPage.vue";

const routes: RouteRecordRaw[] = [
  { path: "/", name: "Dashboard", component: DashboardPage },
  { path: "/messages", name: "Messages", component: MessagesPage },
  { path: "/events", name: "Events", component: EventsPage },
  { path: "/domains", name: "Domains", component: DomainsPage },
  { path: "/templates", name: "Templates", component: TemplatesPage },
  { path: "/mailing-lists", name: "MailingLists", component: MailingListsPage },
  { path: "/routes", name: "Routes", component: RoutesPage },
  { path: "/suppressions", name: "Suppressions", component: SuppressionsPage },
  { path: "/webhooks", name: "Webhooks", component: WebhooksPage },
  { path: "/settings", name: "Settings", component: SettingsPage },
  { path: "/trigger-events", name: "TriggerEvents", component: TriggerEventsPage },
  { path: "/simulate-inbound", name: "SimulateInbound", component: SimulateInboundPage },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

export default router;
