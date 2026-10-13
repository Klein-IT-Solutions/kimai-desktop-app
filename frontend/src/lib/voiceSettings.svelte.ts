import { VoiceService } from "../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";

/** Whether a transcription engine is configured (enables the mic button). */
export const voiceEnabled = $state({ value: false });

export function initVoiceSettings() {
  VoiceService.GetSettings()
    .then((s) => (voiceEnabled.value = s.engine !== ""))
    .catch(() => {});
}
