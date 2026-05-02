const params = new URLSearchParams(location.search);
const state = {
  requests: [],
  selectedRequest: null,
  filterMethod: "ALL",
  searchQuery: "",
  tab: "request",
  session: params.get("session"),
  token: params.get("token"),
  sessionInfo: null,
  ws: null,
  reconnectTimer: null,
  timerId: null,
  loading: true,
  error: "",
  replayBusy: false,
};

const elements = {
  tunnelURL: document.getElementById("tunnel-url"),
  connectionStatus: document.getElementById("connection-status"),
  connectionDot: document.getElementById("connection-dot"),
  requestCount: document.getElementById("request-count"),
  sessionDuration: document.getElementById("session-duration"),
  requestList: document.getElementById("request-list"),
  listState: document.getElementById("list-state"),
  selectedTitle: document.getElementById("selected-title"),
  selectedMeta: document.getElementById("selected-meta"),
  leftPanel: document.getElementById("left-panel"),
  rightPanel: document.getElementById("right-panel"),
  errorBanner: document.getElementById("error-banner"),
  replayStatus: document.getElementById("replay-status"),
  footerStatus: document.getElementById("footer-status"),
  copyCurl: document.getElementById("copy-curl"),
  replay: document.getElementById("replay"),
  search: document.getElementById("search"),
};

const methodButtons = [...document.querySelectorAll("[data-method]")];
const tabButtons = [...document.querySelectorAll("[data-tab]")];

const apiBase = (path) =>
  `${path}?session=${encodeURIComponent(state.session || "")}&token=${encodeURIComponent(state.token || "")}`;

const formatDate = (value) => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "unknown time";
  return date.toLocaleString();
};

const formatDuration = (ms) => {
  if (!Number.isFinite(ms)) return "0ms";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(ms < 10000 ? 2 : 1)}s`;
};

const statusClass = (code) => {
  if (code >= 500) return "status-bad";
  if (code >= 400) return "status-warn";
  return "status-ok";
};

const parseHeaderJSON = (raw) => {
  if (!raw) return {};
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
};

const asArray = (value) => (Array.isArray(value) ? value : []);

const prettyPayload = (raw) => {
  if (!raw) return "";
  const trimmed = String(raw).trim();
  if (!trimmed) return "";
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2);
  } catch {
    return String(raw);
  }
};

const isProbablyJSON = (raw) => {
  if (!raw) return false;
  const trimmed = String(raw).trim();
  return trimmed.startsWith("{") || trimmed.startsWith("[");
};

const clearNode = (node) => {
  while (node.firstChild) node.firstChild.remove();
};

const make = (tag, options = {}) => {
  const node = document.createElement(tag);
  if (options.className) node.className = options.className;
  if (options.text !== undefined) node.textContent = options.text;
  if (options.attrs) {
    Object.entries(options.attrs).forEach(([key, value]) =>
      node.setAttribute(key, value),
    );
  }
  return node;
};

const showError = (message) => {
  state.error = message || "";
  renderBanner();
};

const renderBanner = () => {
  elements.errorBanner.textContent = state.error;
  elements.errorBanner.classList.toggle("visible", Boolean(state.error));
  elements.footerStatus.textContent = state.error ? "attention required" : "";
};

const setConnectionState = (status) => {
  elements.connectionStatus.textContent = status;
  elements.connectionDot.className = "connection-dot";
  if (status === "connected") {
    elements.connectionDot.classList.add("connected");
  } else if (status === "connecting") {
    elements.connectionDot.classList.add("connecting");
  }
};

const filteredRequests = () => {
  const query = state.searchQuery.trim().toLowerCase();
  return asArray(state.requests).filter((request) => {
    if (state.filterMethod !== "ALL" && request.method !== state.filterMethod) {
      return false;
    }
    if (!query) return true;
    const haystack = [
      request.method,
      request.path,
      request.query,
      request.body,
      request.response_body,
      request.headers,
      request.response_headers,
      String(request.response_status),
    ]
      .join("\n")
      .toLowerCase();
    return haystack.includes(query);
  });
};

const renderRequestList = () => {
  clearNode(elements.requestList);
  const requests = filteredRequests();
  elements.requestCount.textContent = `${requests.length} request${requests.length === 1 ? "" : "s"}`;

  if (state.loading) {
    elements.listState.textContent = "Loading requests...";
    return;
  }

  if (!state.sessionInfo) {
    elements.listState.textContent = "No authorized session loaded.";
    return;
  }

  if (!requests.length) {
    elements.listState.textContent = state.searchQuery
      ? "No requests match the current filters."
      : "No captured requests yet.";
    return;
  }

  elements.listState.textContent = `${requests.length} visible result${requests.length === 1 ? "" : "s"}`;

  requests.forEach((request) => {
    const button = make("button", {
      className: `request-item${state.selectedRequest && state.selectedRequest.id === request.id ? " active" : ""}`,
      attrs: {
        type: "button",
        role: "option",
        "aria-selected":
          state.selectedRequest && state.selectedRequest.id === request.id
            ? "true"
            : "false",
      },
    });

    const line = make("div", { className: "request-line" });
    line.append(
      make("span", {
        className: `method method-${request.method}`,
        text: request.method,
      }),
      make("span", {
        className: "request-path",
        text: request.path + (request.query ? `?${request.query}` : ""),
      }),
      make("span", {
        className: `status-code ${statusClass(request.response_status)}`,
        text: String(request.response_status),
      }),
      make("span", {
        className: "duration subtle",
        text: formatDuration(request.duration_ms),
      }),
    );

    const meta = make("div", {
      className: "request-subline",
      text: `${formatDate(request.created_at)}${request.replay ? " | replayed" : ""}`,
    });

    button.append(line, meta);
    button.addEventListener("click", () => openRequest(request.id));
    elements.requestList.appendChild(button);
  });
};

const renderPanelSection = (title, content) => {
  const wrapper = make("div", { className: "split" });
  const header = make("div", { className: "detail-header" });
  header.append(
    make("span", { className: "detail-title", text: title }),
    make("span", { className: "subtle", text: content.kind }),
  );
  const block = make("div", { className: "code-block" });
  block.appendChild(make("pre", { text: content.value || "none" }));
  wrapper.append(header, block);
  return wrapper;
};

const renderEmptyPanels = (leftMessage, rightMessage) => {
  clearNode(elements.leftPanel);
  clearNode(elements.rightPanel);
  elements.leftPanel.appendChild(
    make("div", { className: "empty-state", text: leftMessage }),
  );
  elements.rightPanel.appendChild(
    make("div", { className: "empty-state", text: rightMessage }),
  );
};

const renderRequestTab = (request) => {
  elements.leftPanel.appendChild(
    renderPanelSection("Request headers", {
      kind: "headers",
      value: prettyPayload(request.headers),
    }),
  );
  elements.rightPanel.appendChild(
    renderPanelSection("Request body", {
      kind: isProbablyJSON(request.body) ? "json" : "raw",
      value: prettyPayload(request.body),
    }),
  );
};

const renderResponseTab = (request) => {
  elements.leftPanel.appendChild(
    renderPanelSection(`Response ${request.response_status}`, {
      kind: "headers",
      value: prettyPayload(request.response_headers),
    }),
  );
  elements.rightPanel.appendChild(
    renderPanelSection(
      request.replay ? "Latest replay response" : "Response body",
      {
        kind: (() => {
          const responseBody = request.replay
            ? request.replay.response_body
            : request.response_body;

          if (request.replay && isProbablyJSON(request.replay.response_body)) {
            return "json";
          }

          return isProbablyJSON(responseBody) ? "json" : "raw";
        })(),
        value: request.replay
          ? prettyPayload(request.replay.response_body)
          : prettyPayload(request.response_body),
      },
    ),
  );
  if (request.replay) {
    elements.rightPanel.appendChild(
      renderPanelSection("Replay response headers", {
        kind: "headers",
        value: prettyPayload(request.replay.response_headers),
      }),
    );
  } else {
    elements.rightPanel.appendChild(
      renderPanelSection("Original response body", {
        kind: isProbablyJSON(request.response_body) ? "json" : "raw",
        value: prettyPayload(request.response_body),
      }),
    );
  }
};

const renderMutateTab = (request) => {
  const leftWrap = make("div", { className: "split" });
  const rightWrap = make("div", { className: "split" });

  const headersField = make("div", { className: "field" });
  headersField.append(
    make("label", { attrs: { for: "mutate-headers" }, text: "Headers JSON" }),
  );
  const headersArea = make("textarea", {
    attrs: { id: "mutate-headers", spellcheck: "false" },
  });
  headersArea.value = prettyPayload(request.headers);
  headersField.append(headersArea);
  headersField.append(
    make("div", {
      className: "help",
      text: "Use a flat JSON object or the original captured headers.",
    }),
  );

  const bodyField = make("div", { className: "field" });
  bodyField.append(
    make("label", { attrs: { for: "mutate-body" }, text: "Body" }),
  );
  const bodyArea = make("textarea", {
    attrs: { id: "mutate-body", spellcheck: "false" },
  });
  bodyArea.value = request.body || "";
  bodyField.append(bodyArea);
  bodyField.append(
    make("div", {
      className: "help",
      text: "Replay uses these edited values without modifying the original capture.",
    }),
  );

  leftWrap.append(headersField);
  rightWrap.append(bodyField);
  elements.leftPanel.append(leftWrap);
  elements.rightPanel.append(rightWrap);
};

const renderRequestDetail = () => {
  const request = state.selectedRequest;
  clearNode(elements.leftPanel);
  clearNode(elements.rightPanel);

  if (!request) {
    elements.selectedTitle.textContent = "Select a request";
    elements.selectedMeta.textContent =
      "Live traffic appears here as requests arrive.";
    elements.copyCurl.disabled = true;
    elements.replay.disabled = true;
    renderEmptyPanels("No request selected.", "Select a request to inspect it.");
    return;
  }

  elements.copyCurl.disabled = false;
  elements.replay.disabled = state.replayBusy;
  elements.selectedTitle.textContent = `${request.method} ${request.path}`;
  elements.selectedMeta.textContent = [
    `${request.response_status}`,
    formatDuration(request.duration_ms),
    formatDate(request.created_at),
  ].join(" | ");

  if (state.tab === "request") {
    renderRequestTab(request);
    return;
  }

  if (state.tab === "response") {
    renderResponseTab(request);
    return;
  }

  if (state.tab === "mutate") {
    renderMutateTab(request);
    return;
  }

  renderEmptyPanels("Select a detail tab.", "Select a detail tab.");
};

const render = () => {
  renderBanner();
  renderRequestList();
  renderRequestDetail();
};

const updateTimer = () => {
  const startedAt = state.sessionInfo?.started_at;
  if (!startedAt) {
    elements.sessionDuration.textContent = "00:00:00";
    return;
  }
  const started = new Date(startedAt).getTime();
  if (Number.isNaN(started)) {
    elements.sessionDuration.textContent = "00:00:00";
    return;
  }
  const delta = Math.max(0, Math.floor((Date.now() - started) / 1000));
  const h = String(Math.floor(delta / 3600)).padStart(2, "0");
  const m = String(Math.floor((delta % 3600) / 60)).padStart(2, "0");
  const s = String(delta % 60).padStart(2, "0");
  elements.sessionDuration.textContent = `${h}:${m}:${s}`;
};

const requestJSON = async (path, init) => {
  const response = await fetch(apiBase(path), init);
  if (!response.ok) {
    const message = await response.text().catch(() => response.statusText);
    throw new Error(message || `Request failed with ${response.status}`);
  }
  return response.json();
};

const ensureSelectedRequest = (requests) => {
  requests = asArray(requests);
  if (!requests.length) return null;
  if (!state.selectedRequest) return requests[0];
  return requests.find((item) => item.id === state.selectedRequest.id) || requests[0];
};

const loadInitial = async () => {
  state.loading = true;
  render();
  try {
    const sessions = await requestJSON("/api/sessions");
    let current = sessions.find((item) => item.id === state.session);
    if (!current && sessions.length) current = sessions[0];
    if (!current) {
      throw new Error("No session available for this dashboard token.");
    }

    state.session = current.id;
    state.sessionInfo = current;
    elements.tunnelURL.textContent = `${location.origin}/t/${current.subdomain}`;

    state.requests = asArray(await requestJSON("/api/requests"));
    state.selectedRequest = ensureSelectedRequest(state.requests);
    state.loading = false;
    showError("");
    render();
    connectWS();
    if (!state.timerId) {
      state.timerId = globalThis.setInterval(updateTimer, 1000);
    }
    updateTimer();
  } catch (error) {
    state.loading = false;
    showError(error.message || "Failed to load dashboard data.");
    render();
  }
};

const openRequest = async (id) => {
  try {
    const request = await requestJSON(`/api/requests/${id}`);
    state.selectedRequest = request;
    showError("");
    renderRequestDetail();
    renderRequestList();
  } catch (error) {
    showError(error.message || "Failed to load request details.");
  }
};

const scheduleReconnect = () => {
  if (state.reconnectTimer) return;
  state.reconnectTimer = globalThis.setTimeout(() => {
    state.reconnectTimer = null;
    connectWS();
  }, 3000);
};

const connectWS = () => {
  if (!state.session) return;
  if (state.ws && state.ws.readyState <= WebSocket.OPEN) return;

  if (state.reconnectTimer) {
    globalThis.clearTimeout(state.reconnectTimer);
    state.reconnectTimer = null;
  }

  setConnectionState("connecting");
  const protocol = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(
    `${protocol}//${location.host}/ws/events?session=${encodeURIComponent(state.session)}&token=${encodeURIComponent(state.token || "")}`,
  );
  state.ws = ws;

  ws.onopen = () => {
    if (state.ws !== ws) return;
    setConnectionState("connected");
  };

  ws.onclose = () => {
    if (state.ws === ws) {
      state.ws = null;
    }
    setConnectionState("offline");
    scheduleReconnect();
  };

  ws.onerror = () => {
    showError("Live updates disconnected. The dashboard will retry automatically.");
  };

  ws.onmessage = (event) => {
    let data;
    try {
      data = JSON.parse(event.data);
    } catch {
      return;
    }

    if (data.type !== "new_request" || !data.payload) return;
    const nextRequests = [
      data.payload,
      ...asArray(state.requests).filter((item) => item.id !== data.payload.id),
    ];
    state.requests = nextRequests;
    if (!state.selectedRequest) {
      state.selectedRequest = data.payload;
    }
    render();
  };
};

const runReplay = async () => {
  if (!state.selectedRequest || state.replayBusy) return;

  let headers = {};
  let body = state.selectedRequest.body || "";

  if (state.tab === "mutate") {
    const headersInput = document.getElementById("mutate-headers");
    const bodyInput = document.getElementById("mutate-body");
    try {
      headers = JSON.parse(headersInput?.value || "{}");
    } catch {
      showError("Mutated headers must be valid JSON.");
      return;
    }
    body = bodyInput?.value || "";
  }

  state.replayBusy = true;
  elements.replay.disabled = true;

  try {
    const replay = await requestJSON("/api/replay", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        request_id: state.selectedRequest.id,
        headers,
        body,
      }),
    });
    elements.replayStatus.textContent = `last replay ${replay.response_status} in ${formatDuration(replay.duration_ms)}`;
    state.tab = "response";
    syncTabs();
    await openRequest(state.selectedRequest.id);
    showError("");
  } catch (error) {
    showError(error.message || "Replay failed.");
  } finally {
    state.replayBusy = false;
    elements.replay.disabled = false;
  }
};

const escapeForSingleQuotes = (value) =>
  String(value || "").replaceAll("'", "'\"'\"'");

const copyCurl = async () => {
  if (!state.selectedRequest || !state.sessionInfo) return;
  const request = state.selectedRequest;
  const querySuffix = request.query ? "?" + request.query : "";
  const url = `${location.origin}/t/${state.sessionInfo.subdomain}${request.path}${querySuffix}`;
  const headers = parseHeaderJSON(request.headers);
  const headerParts = Object.entries(headers)
    .flatMap(([key, values]) =>
      (Array.isArray(values) ? values : [values]).map(
        (value) => `-H '${escapeForSingleQuotes(key + ": " + value)}'`,
      ),
    )
    .join(" ");
  const bodyPart = request.body
    ? ` --data '${escapeForSingleQuotes(request.body)}'`
    : "";
  const headerSegment = headerParts ? " " + headerParts : "";
  const curl = `curl -X ${request.method} '${url}'${headerSegment}${bodyPart}`;
  try {
    await navigator.clipboard.writeText(curl);
    elements.replayStatus.textContent = "curl copied to clipboard";
  } catch {
    showError("Clipboard access failed. Copy curl is only available in a secure browser context.");
  }
};

const syncTabs = () => {
  tabButtons.forEach((button) => {
    const active = button.dataset.tab === state.tab;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", active ? "true" : "false");
  });
};

methodButtons.forEach((button) => {
  button.addEventListener("click", () => {
    state.filterMethod = button.dataset.method;
    methodButtons.forEach((item) =>
      item.classList.toggle("active", item === button),
    );
    renderRequestList();
  });
});

tabButtons.forEach((button) => {
  button.addEventListener("click", () => {
    state.tab = button.dataset.tab;
    syncTabs();
    renderRequestDetail();
  });
});

elements.search.addEventListener("input", (event) => {
  state.searchQuery = event.target.value;
  renderRequestList();
});

elements.copyCurl.addEventListener("click", copyCurl);
elements.replay.addEventListener("click", runReplay);

globalThis.addEventListener("beforeunload", () => {
  if (state.reconnectTimer) {
    globalThis.clearTimeout(state.reconnectTimer);
  }
  if (state.timerId) {
    globalThis.clearInterval(state.timerId);
  }
  if (state.ws) {
    state.ws.close();
  }
});

setConnectionState("offline");
syncTabs();
render();
await loadInitial();
