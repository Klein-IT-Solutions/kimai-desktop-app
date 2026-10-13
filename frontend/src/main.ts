import "@fontsource-variable/inter";
import "./app.css";
import { mount } from "svelte";
import App from "./App.svelte";
import { initTheme } from "./lib/theme";
import { initConnection } from "./lib/connection.svelte";
import { initPlatform } from "./lib/platform";
import { initTracker } from "./lib/tracker.svelte";
import { initShell } from "./lib/shell.svelte";
import { initVoice } from "./lib/voice.svelte";
import { initVoiceSettings } from "./lib/voiceSettings.svelte";
import { initAssistant } from "./lib/assistant.svelte";

initTheme();
initConnection();
initTracker();
initShell();
initVoice();
initVoiceSettings();
initAssistant();

initPlatform().finally(() => {
  mount(App, { target: document.getElementById("app")! });
});
