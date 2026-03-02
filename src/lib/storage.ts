export function loadFromStorage<T>(key: string, fallback: T): T {
  try {
    const saved = localStorage.getItem(key);
    if (saved) return JSON.parse(saved) as T;
  } catch {
    /* corrupt data — fall through */
  }
  return fallback;
}

export function saveToStorage(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch (e) {
    console.error(`[PacketCode] Failed to save to localStorage key "${key}":`, e);
  }
}

/** Get approximate localStorage usage in bytes. */
export function getStorageUsage(): { used: number; keys: number } {
  let used = 0;
  let keys = 0;
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (key) {
      keys++;
      used += key.length + (localStorage.getItem(key)?.length ?? 0);
    }
  }
  return { used: used * 2, keys }; // *2 for UTF-16
}

export function removeFromStorage(key: string): void {
  localStorage.removeItem(key);
}

export function generateId(prefix: string, length = 8): string {
  return `${prefix}_${Date.now()}_${Math.random().toString(36).slice(2, 2 + length)}`;
}

export function parseJsonFromResponse(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    const fenceMatch = text.match(/```(?:json)?\s*([\s\S]*?)```/);
    if (fenceMatch) return JSON.parse(fenceMatch[1].trim());
    const arrMatch = text.match(/\[[\s\S]*\]/);
    if (arrMatch) return JSON.parse(arrMatch[0]);
    const objMatch = text.match(/\{[\s\S]*\}/);
    if (objMatch) return JSON.parse(objMatch[0]);
    throw new Error("No valid JSON found in response");
  }
}
