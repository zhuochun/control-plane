export type WatchHealth = {
  watch_id: string;
  next_due_at: string;
  due_reason: string;
  last_status?: string;
  last_attempt_at?: string;
  last_success_at?: string;
  last_error?: string;
};

export type ServiceStatus = {
  version: string;
  database: string;
  health: {
    last_run: { id: string; status: string; started_at: string; summary?: string } | null;
    active_run: { id: string; started_at: string; selected_count: number; submitted_count: number } | null;
    due_count: number;
    item_counts: { all: number; attention: number; todo: number };
    watches: WatchHealth[];
    setup: { user_context_set: boolean; has_interest: boolean; has_watcher: boolean; done: boolean };
  };
};
