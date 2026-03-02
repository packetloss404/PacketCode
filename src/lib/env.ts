/** Whether the app is running in development mode. */
export const isDev = import.meta.env.DEV;

/** Whether the app is running in production mode. */
export const isProd = import.meta.env.PROD;

/** Log only in development mode. */
export function devLog(...args: unknown[]): void {
  if (isDev) {
    console.log("[PacketCode:dev]", ...args);
  }
}
