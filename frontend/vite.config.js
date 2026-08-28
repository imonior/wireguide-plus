import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    svelte({
      // Desktop client: silence the two backdrop-related a11y warnings.
      // Backdrops are plain overlay divs whose click/mousedown closes the
      // modal; assigning role="button" would make screen readers announce
      // the whole overlay as a button, which is worse. The modal dialogs
      // themselves stay fully ARIA-compliant (role="dialog" aria-modal
      // tabindex="-1").
      onwarn(warning, warn) {
        if (warning.code === "a11y_click_events_have_key_events") return;
        if (warning.code === "a11y_no_static_element_interactions") return;
        // The pane-resizer is a real keyboard-operable separator
        // (role="separator" tabindex="0" on:keydown), but Svelte's static
        // a11y rules do not treat `separator` as an interactive role.
        if (warning.code === "a11y_no_noninteractive_tabindex") return;
        if (warning.code === "a11y_no_noninteractive_element_interactions") return;
        warn(warning);
      },
    }),
    wails("./bindings"),
  ],
  build: {
    // WebKitGTK versions shipped by supported Linux distributions lag the
    // evergreen browsers Vite targets by default. Transpile modern syntax so
    // the application mounts on those embedded WebKit runtimes as well.
    target: "safari13",
    // Let Rolldown derive safe chunk boundaries. The previous forced chunk
    // graph created circular startup imports under Vite 8 and left WebKitGTK
    // with a blank window before the Svelte application could mount.
    chunkSizeWarningLimit: 600,
  },
});
