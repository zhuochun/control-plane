import {
  createContext,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
} from "@mui/material";

export type ShortcutCommands = Partial<
  Record<
    | "next"
    | "previous"
    | "insert"
    | "seen"
    | "todo"
    | "done"
    | "remind"
    | "back"
    | "saveExit"
    | "saveStay",
    () => void
  >
>;
const preferenceKey = "aicp.vim-shortcuts";
type KeyboardContext = {
  enabled: boolean;
  setEnabled: (value: boolean) => void;
  showHelp: () => void;
  announce: (message: string) => void;
  register: (commands: ShortcutCommands) => () => void;
};
const Context = createContext<KeyboardContext | null>(null);

export function useKeyboard() {
  const value = useContext(Context);
  if (!value) throw new Error("KeyboardProvider is required");
  return value;
}

// One listener dispatches to the currently mounted owners; registration cleanup
// removes a reader immediately when its Item changes.
export function useShortcutCommands(commands: ShortcutCommands) {
  const { register } = useKeyboard();
  useLayoutEffect(() => register(commands), [register, commands]);
}

export function KeyboardProvider({ children }: { children: ReactNode }) {
  const [enabled, updateEnabled] = useState(() => {
    try {
      return localStorage.getItem(preferenceKey) === "true";
    } catch {
      return false;
    }
  });
  const [help, setHelp] = useState(false);
  const [status, setStatus] = useState({ sequence: 0, message: "" });
  const owners = useRef(new Set<ShortcutCommands>());
  const [register] = useState(() => (commands: ShortcutCommands) => {
    owners.current.add(commands);
    return () => {
      owners.current.delete(commands);
    };
  });
  const searchOrigin = useRef<HTMLElement | null>(null);
  const composing = useRef(false);
  const setEnabled = (value: boolean) => {
    updateEnabled(value);
    try {
      localStorage.setItem(preferenceKey, String(value));
    } catch {
      /* Session preference still works. */
    }
  };

  useEffect(() => {
    const start = () => {
      composing.current = true;
    };
    const end = () => {
      composing.current = false;
    };
    const keydown = (event: KeyboardEvent) => {
      if (
        !enabled ||
        event.defaultPrevented ||
        event.isComposing ||
        composing.current ||
        event.keyCode === 229
      )
        return;
      // MUI owns modal keys and focus restoration, including Escape.
      if (
        Array.from(
          document.querySelectorAll<HTMLElement>(
            '[role="dialog"], [role="menu"], [role="listbox"]',
          ),
        ).some((el) => el.getClientRects().length > 0)
      )
        return;
      const target = event.target instanceof HTMLElement ? event.target : null;
      const note = target?.matches("[data-vim-note]");
      const saveChord =
        note &&
        event.key === "Enter" &&
        (event.ctrlKey || event.metaKey) &&
        !event.altKey &&
        !event.shiftKey;
      if (
        !saveChord &&
        (event.ctrlKey ||
          event.metaKey ||
          event.altKey ||
          (event.shiftKey && event.key !== "?"))
      )
        return;
      const supported = /^\/(?:todos|library|items\/[^/]+)?$/.test(
        window.location.pathname,
      );
      if (!supported) return;
      let command: keyof ShortcutCommands | undefined;
      if (note && (saveChord || event.key === "Escape"))
        command = saveChord ? "saveStay" : "saveExit";
      else if (
        target?.matches(".portal-search input") &&
        event.key === "Escape"
      ) {
        event.preventDefault();
        if (searchOrigin.current?.isConnected) searchOrigin.current.focus();
        if (document.activeElement === target)
          document.querySelector<HTMLElement>("[data-reader-item]")?.focus();
        if (document.activeElement === target)
          document.querySelector<HTMLElement>(".workspace-list")?.focus();
        return;
      } else if (
        target?.closest(
          'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="combobox"], [role="slider"], [role="spinbutton"], [role="tree"], [role="grid"], [role="tablist"]',
        )
      )
        return;
      else {
        if (event.key === "/") {
          if (event.repeat) return;
          const input = document.querySelector<HTMLInputElement>(
            ".portal-search input",
          );
          if (input) {
            event.preventDefault();
            searchOrigin.current = document.activeElement as HTMLElement;
            input.focus();
            input.select();
          }
          return;
        }
        if (event.key === "?") {
          if (!event.repeat) {
            event.preventDefault();
            setHelp(true);
          }
          return;
        }
        if (event.key === "n") {
          if (!event.repeat) {
            event.preventDefault();
            document
              .querySelector<HTMLButtonElement>("[data-inbox-open]")
              ?.click();
          }
          return;
        }
        const bindings: Record<string, keyof ShortcutCommands> = {
          j: "next",
          k: "previous",
          i: "insert",
          r: "seen",
          t: "todo",
          d: "done",
          s: "remind",
          Escape: "back",
        };
        command = bindings[event.key];
      }
      if (
        !command ||
        (event.repeat && command !== "next" && command !== "previous")
      )
        return;
      const handler = Array.from(owners.current)
        .reverse()
        .find((owner) => owner[command])?.[command];
      if (handler) {
        event.preventDefault();
        handler();
      }
    };
    window.addEventListener("keydown", keydown);
    window.addEventListener("compositionstart", start);
    window.addEventListener("compositionend", end);
    return () => {
      window.removeEventListener("keydown", keydown);
      window.removeEventListener("compositionstart", start);
      window.removeEventListener("compositionend", end);
    };
  }, [enabled]);

  return (
    <Context.Provider
      value={{
        enabled,
        setEnabled,
        showHelp: () => setHelp(true),
        announce: (message) =>
          setStatus((current) => ({ sequence: current.sequence + 1, message })),
        register,
      }}
    >
      {children}
      <span className="visually-hidden" role="status" aria-atomic="true">
        <span key={status.sequence}>{status.message}</span>
      </span>
      <Dialog
        open={help}
        onClose={() => setHelp(false)}
        fullWidth
        maxWidth="sm"
        aria-labelledby="shortcut-help-title"
      >
        <DialogTitle id="shortcut-help-title">Keyboard shortcuts</DialogTitle>
        <DialogContent>
          <p>
            Vim shortcuts are {enabled ? "on" : "off"}. Change this browser's
            preference in Preferences.
          </p>
          <table className="shortcut-help">
            <thead>
              <tr>
                <th>Key</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <kbd>j</kbd> / <kbd>k</kbd>
                </td>
                <td>Next / previous Item in the queue</td>
              </tr>
              <tr>
                <td>
                  <kbd>i</kbd>
                </td>
                <td>Edit Your note</td>
              </tr>
              <tr>
                <td>
                  <kbd>Esc</kbd> in Your note
                </td>
                <td>Save and exit after success</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl/Cmd+Enter</kbd>
                </td>
                <td>Save Your note and keep editing</td>
              </tr>
              <tr>
                <td>
                  <kbd>r</kbd>
                </td>
                <td>Mark seen</td>
              </tr>
              <tr>
                <td>
                  <kbd>t</kbd> / <kbd>d</kbd>
                </td>
                <td>Add or reopen Todo / mark open Todo done</td>
              </tr>
              <tr>
                <td>
                  <kbd>s</kbd>
                </td>
                <td>Set or change reminder</td>
              </tr>
              <tr>
                <td>
                  <kbd>/</kbd>
                </td>
                <td>Search</td>
              </tr>
              <tr>
                <td>
                  <kbd>n</kbd>
                </td>
                <td>Add to inbox</td>
              </tr>
              <tr>
                <td>
                  <kbd>?</kbd>
                </td>
                <td>Shortcut help</td>
              </tr>
              <tr>
                <td>
                  <kbd>Esc</kbd> elsewhere
                </td>
                <td>
                  Close overlay, leave search, or return to the queue on narrow
                  screens
                </td>
              </tr>
            </tbody>
          </table>
          <p>
            Item commands require a loaded Item. Queue navigation is unavailable
            on standalone Item detail. Ordinary typing and form controls keep
            their own keys.
          </p>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setHelp(false)}>Close</Button>
        </DialogActions>
      </Dialog>
    </Context.Provider>
  );
}

export function KeyHint({ children }: { children: string }) {
  const { enabled } = useKeyboard();
  return enabled ? (
    <kbd className="key-hint" aria-hidden="true">
      {children}
    </kbd>
  ) : null;
}
