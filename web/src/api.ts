export class APIError extends Error {
  constructor(
    message: string,
    public code?: string,
    public details?: Record<string, unknown>,
  ) {
    super(message);
  }
}

export async function api<T>(
  path: string,
  body?: unknown,
  method = "PATCH",
): Promise<T> {
  const response = await fetch(
    `/api/v1${path}`,
    body === undefined
      ? undefined
      : {
          method,
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        },
  );
  const result = await response.json();
  if (!response.ok)
    throw new APIError(
      result.error?.message ?? "The server could not complete this request.",
      result.error?.code,
      result.error?.details,
    );
  return result as T;
}

export async function collection<T>(path: string): Promise<T[]> {
  const items: T[] = [];
  let cursor: string | null = null;
  do {
    const query = new URLSearchParams({
      limit: "100",
      ...(cursor ? { cursor } : {}),
    });
    const page: { items: T[]; next_cursor: string | null } = await api(
      `${path}${path.includes("?") ? "&" : "?"}${query}`,
    );
    items.push(...page.items);
    cursor = page.next_cursor;
  } while (cursor);
  return items;
}
