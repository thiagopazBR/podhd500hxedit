// Parameter writes: one request in flight, the newest value wins (sliders
// send on every move; values made while a request runs are coalesced).
import { post } from "../../api";

const sender: { busy: boolean; next: { url: string; body: unknown } | null } = { busy: false, next: null };

export async function queueSend(url: string, body: unknown, onError: (msg: string) => void): Promise<void> {
  sender.next = { url, body };
  if (sender.busy) return;
  sender.busy = true;
  try {
    while (sender.next) {
      const { url: u, body: b } = sender.next;
      sender.next = null;
      await post(u, b);
    }
  } catch (e) {
    sender.next = null;
    onError(`Couldn't change the parameter: ${(e as Error).message}`);
  } finally {
    sender.busy = false;
  }
}
