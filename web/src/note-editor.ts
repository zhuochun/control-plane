import { useState, useSyncExternalStore } from "react";
import {
  useIsMutating,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { api, APIError } from "./api";
import type { Item } from "./items";

type BaseNote = { version: number; text: string; inputId?: string };
type Attempt = {
  request_id: string;
  actor: "user";
  expected_state_version: number;
  user_note: string;
};
type Edit = {
  draft: string | null;
  base?: BaseNote;
  attempt?: Attempt;
  pending: boolean;
  error: Error | null;
  saved: boolean;
};
const empty: Edit = { draft: null, pending: false, error: null, saved: false };
const baseNote = (item: Item): BaseNote => ({
  version: item.state_version,
  text: item.user_note,
  inputId: item.pending_inputs?.find((input) => input.kind === "note")?.id,
});

// Owned by the mounted workspace/detail, not by a keyed reader. Draft, conflict
// base, pending attempt, and retry identity therefore travel together.
export class NoteEdits {
  private edits = new Map<string, Edit>();
  private listeners = new Set<() => void>();
  private revision = 0;
  constructor(private cache: QueryClient) {}
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  get = (id: string) => this.edits.get(id) ?? empty;
  getRevision = () => this.revision;
  private put(id: string, edit: Edit) {
    this.edits.set(id, edit);
    this.revision++;
    this.listeners.forEach((listener) => listener());
  }
  change(item: Item, value: string) {
    const edit = this.get(item.id);
    this.put(item.id, {
      ...edit,
      draft: value,
      base: edit.base ?? baseNote(item),
      attempt:
        edit.pending || edit.attempt?.user_note === value
          ? edit.attempt
          : undefined,
      error: null,
      saved: false,
    });
  }
  async save(item: Item): Promise<boolean> {
    let edit = this.get(item.id);
    if (
      edit.pending ||
      this.cache.isMutating({ mutationKey: ["item-write", item.id] })
    )
      return false;
    if (edit.draft === null || edit.draft === item.user_note) {
      this.put(item.id, empty);
      return true;
    }
    // A definite version conflict can be retried explicitly against refreshed
    // state. An uncertain transport outcome keeps the exact original request.
    const base =
      edit.error instanceof APIError && edit.error.code === "state_conflict"
        ? baseNote(item)
        : edit.base;
    const sameNote =
      !base ||
      (base.text === item.user_note && base.inputId === baseNote(item).inputId);
    const attempt =
      edit.attempt &&
      !(edit.error instanceof APIError && edit.error.code === "state_conflict")
        ? edit.attempt
        : {
            request_id: crypto.randomUUID(),
            actor: "user" as const,
            expected_state_version: sameNote
              ? item.state_version
              : base!.version,
            user_note: edit.draft,
          };
    edit = { ...edit, base, attempt, pending: true, error: null, saved: false };
    this.put(item.id, edit);
    const mutation = this.cache.getMutationCache().build(this.cache, {
      mutationKey: ["item-write", item.id],
      retry: false,
      mutationFn: () => api<Item>(`/items/${item.id}/note`, attempt, "PUT"),
    });
    try {
      const updated = await mutation.execute(undefined);
      this.cache.setQueryData(["item", item.id], updated);
      void this.cache.invalidateQueries({ queryKey: ["items"] });
      const current = this.get(item.id);
      this.put(
        item.id,
        current.draft === attempt.user_note
          ? { ...empty, saved: true }
          : { ...empty, draft: current.draft, base: baseNote(updated) },
      );
      return true;
    } catch (error) {
      this.put(item.id, {
        ...this.get(item.id),
        attempt:
          this.get(item.id).draft === attempt.user_note ? attempt : undefined,
        pending: false,
        error: error instanceof Error ? error : new Error(String(error)),
      });
      void this.cache.invalidateQueries({ queryKey: ["item", item.id] });
      return false;
    }
  }
}

export function useNoteEdits() {
  const cache = useQueryClient();
  const [edits] = useState(() => new NoteEdits(cache));
  useSyncExternalStore(edits.subscribe, edits.getRevision);
  return edits;
}

export function useNoteEdit(edits: NoteEdits, id: string) {
  return useSyncExternalStore(edits.subscribe, () => edits.get(id));
}

export function useItemPending(id: string) {
  return useIsMutating({ mutationKey: ["item-write", id] }) > 0;
}
