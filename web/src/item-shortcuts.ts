import { useShortcutCommands } from "./keyboard";
import type { Item } from "./items";

export function useItemShortcuts({
  item,
  pending,
  available,
  apply,
  remind,
  save,
}: {
  item?: Item;
  pending: boolean;
  available: boolean;
  apply: (action: object) => void;
  remind: () => void;
  save: () => Promise<boolean>;
}) {
  const ready = item && available && !pending;
  const reader = () =>
    document.querySelector<HTMLElement>(
      `[data-reader-item="${CSS.escape(item?.id ?? "")}"]`,
    );
  const saveWithFocus = async (exit: boolean) => {
    if (!ready) return;
    const origin = document.activeElement;
    const success = await save();
    if (
      success &&
      exit &&
      origin?.isConnected &&
      document.activeElement === origin
    )
      reader()?.focus();
  };
  useShortcutCommands({
    insert: () => {
      if (!ready) return;
      const input = document.getElementById(
        `note-${item.id}`,
      ) as HTMLTextAreaElement | null;
      if (input) {
        input.focus();
        input.setSelectionRange(input.value.length, input.value.length);
      }
    },
    saveExit: () => {
      void saveWithFocus(true);
    },
    saveStay: () => {
      void saveWithFocus(false);
    },
    seen: () => {
      if (ready && item.acknowledged_content_version < item.content_version)
        apply({ type: "acknowledge", content_version: item.content_version });
    },
    todo: () => {
      if (ready && item.todo_state !== "todo")
        apply({ type: "set_todo", state: "todo" });
    },
    done: () => {
      if (ready && item.todo_state === "todo")
        apply({ type: "set_todo", state: "done" });
    },
    remind: () => {
      if (ready) remind();
    },
  });
}
