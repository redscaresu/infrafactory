function normalizeAPIOrigin(apiOrigin: string, fallbackProtocol: string): string {
  if (!apiOrigin) {
    return "";
  }
  if (apiOrigin.startsWith("http://") || apiOrigin.startsWith("https://")) {
    return apiOrigin;
  }
  return `${fallbackProtocol}//${apiOrigin}`;
}

export function buildWSURL(protocol: string, host: string, apiOrigin = ""): string {
  const normalizedOrigin = normalizeAPIOrigin(apiOrigin, protocol);
  const targetURL = new URL(normalizedOrigin || `${protocol}//${host}`);
  const wsProtocol = targetURL.protocol === "https:" ? "wss:" : "ws:";
  return `${wsProtocol}//${targetURL.host}/api/ws`;
}

/**
 * connectWS opens the event socket and reconnects if it drops.
 *
 * `onStatus`, when supplied, reports whether the socket is currently
 * connected. A caller that renders live output needs it: without it, a
 * dropped socket and "nothing has happened yet" look identical, and a
 * page that shows an empty log for both is telling the reader an apply
 * is quiet when the truth is that it is unobserved.
 */
export function connectWS(
  onMessage: (msg: unknown) => void,
  onStatus?: (connected: boolean) => void
): () => void {
  let closed = false;
  let socket: WebSocket | null = null;
  const apiOrigin = import.meta.env.VITE_UI_API_ORIGIN || (window.location.port === "5173" ? `${window.location.protocol}//${window.location.hostname}:4173` : "");

  const connect = () => {
    if (closed) return;
    socket = new WebSocket(buildWSURL(window.location.protocol, window.location.host, apiOrigin));
    socket.onmessage = (event) => {
      try {
        onMessage(JSON.parse(event.data));
      } catch {
        onMessage({ type: "raw", data: event.data });
      }
    };
    socket.onopen = () => onStatus?.(true);
    socket.onerror = () => onStatus?.(false);
    socket.onclose = () => {
      onStatus?.(false);
      if (!closed) {
        setTimeout(connect, 1000);
      }
    };
  };

  connect();

  return () => {
    closed = true;
    socket?.close();
  };
}
