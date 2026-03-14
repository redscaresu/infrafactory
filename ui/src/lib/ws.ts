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

export function connectWS(onMessage: (msg: unknown) => void): () => void {
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
    socket.onerror = () => {};
    socket.onclose = () => {
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
