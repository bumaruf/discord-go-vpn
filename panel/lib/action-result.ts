export type ActionResult<Data extends object = Record<never, never>> =
  ({ ok: true; message: string } & Data) | { ok: false; error: string }

export function resultMessage(result: ActionResult): string {
  return result.ok ? result.message : result.error
}
