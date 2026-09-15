window.__ModuleLoader__.load({
	id: "dsh-interactive-spec",
	factory: (require) => {
		var module = { exports: {} };
		var exports = module.exports;
		Object.defineProperty(exports, Symbol.toStringTag, { value: "Module" });
		//#region \0rolldown/runtime.js
		var __create = Object.create;
		var __defProp = Object.defineProperty;
		var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
		var __getOwnPropNames = Object.getOwnPropertyNames;
		var __getProtoOf = Object.getPrototypeOf;
		var __hasOwnProp = Object.prototype.hasOwnProperty;
		var __copyProps = (to, from, except, desc) => {
			if (from && typeof from === "object" || typeof from === "function") for (var keys = __getOwnPropNames(from), i = 0, n = keys.length, key; i < n; i++) {
				key = keys[i];
				if (!__hasOwnProp.call(to, key) && key !== except) __defProp(to, key, {
					get: ((k) => from[k]).bind(null, key),
					enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable
				});
			}
			return to;
		};
		var __toESM = (mod, isNodeMode, target) => (target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(isNodeMode || !mod || !mod.__esModule || !__hasOwnProp.call(mod, "default") ? __defProp(target, "default", {
			value: mod,
			enumerable: true
		}) : target, mod));
		//#endregion
		let react = require("react");
		react = __toESM(react);
		//#region lib/src/extractor.js
		const CODE_BLOCK_REGEX = /`{3,}[ \t]*(?:json:)?agentflow-spec[^\r\n]*\r?\n([\s\S]*?)`{3,}/gi;
		/**
		* Remove JS comments (// and /* *\/) and trailing commas outside of string literals.
		*/
		function cleanJsonCommentsAndCommas(input) {
			let insideString = false;
			let stringChar = "";
			let isEscaped = false;
			let result = "";
			let i = 0;
			const len = input.length;
			while (i < len) {
				const char = input[i];
				const nextChar = input[i + 1];
				if (insideString) {
					result += char;
					if (isEscaped) isEscaped = false;
					else if (char === "\\") isEscaped = true;
					else if (char === stringChar) insideString = false;
					i++;
					continue;
				}
				if (char === "\"" || char === "'") {
					insideString = true;
					stringChar = char;
					result += char;
					i++;
					continue;
				}
				if (char === "/" && nextChar === "/") {
					i += 2;
					while (i < len && input[i] !== "\n" && input[i] !== "\r") i++;
					continue;
				}
				if (char === "/" && nextChar === "*") {
					i += 2;
					while (i < len && !(input[i] === "*" && input[i + 1] === "/")) i++;
					i += 2;
					continue;
				}
				result += char;
				i++;
			}
			return result.replace(/,\s*([}\]])/g, "$1");
		}
		/**
		* Tolerant JSON parse that cleans comments and trailing commas if raw parse fails.
		*/
		function tolerantJsonParse(raw) {
			const trimmed = raw.trim();
			if (!trimmed) return null;
			try {
				return JSON.parse(trimmed);
			} catch {
				try {
					const sanitized = cleanJsonCommentsAndCommas(trimmed);
					return JSON.parse(sanitized);
				} catch {
					return null;
				}
			}
		}
		/**
		* Validates and normalizes raw parsed object into a clean LiveSpecDoc.
		*/
		function normalizeLiveSpec(rawObj) {
			if (!rawObj || typeof rawObj !== "object" || Array.isArray(rawObj)) return null;
			const record = rawObj;
			if (!Array.isArray(record.tasks)) return null;
			const tasks = [];
			for (const item of record.tasks) {
				if (!item || typeof item !== "object" || Array.isArray(item)) continue;
				const t = item;
				if (t.id === void 0 || t.id === null) continue;
				const task = {
					...t,
					id: String(t.id),
					title: typeof t.title === "string" ? t.title : String(t.id),
					depends_on: Array.isArray(t.depends_on) ? t.depends_on.map(String) : void 0,
					acceptance_criteria: Array.isArray(t.acceptance_criteria) ? t.acceptance_criteria.map(String) : void 0,
					tags: Array.isArray(t.tags) ? t.tags.map(String) : void 0,
					output_files: Array.isArray(t.output_files) ? t.output_files.map(String) : void 0,
					estimated_hours: typeof t.estimated_hours === "number" ? t.estimated_hours : void 0,
					priority: typeof t.priority === "number" ? t.priority : void 0,
					assigned_worker: typeof t.assigned_worker === "string" ? t.assigned_worker : void 0,
					state: typeof t.state === "string" ? t.state : void 0
				};
				tasks.push(task);
			}
			const doc = {
				version: typeof record.version === "string" ? record.version : "1.0.0",
				title: typeof record.title === "string" ? record.title : "Untitled Spec",
				tasks
			};
			if (typeof record.dag_id === "string") doc.dag_id = record.dag_id;
			if (typeof record.namespace_id === "string") doc.namespace_id = record.namespace_id;
			if (typeof record.concurrency === "number") doc.concurrency = record.concurrency;
			if (record.settings && typeof record.settings === "object" && !Array.isArray(record.settings)) doc.settings = record.settings;
			if (record.metadata && typeof record.metadata === "object" && !Array.isArray(record.metadata)) doc.metadata = record.metadata;
			return doc;
		}
		/**
		* Extracts all valid LiveSpecDoc blocks from Markdown content.
		*/
		function extractAllSpecsFromMarkdown(markdown) {
			if (!markdown || typeof markdown !== "string") return [];
			const specs = [];
			const regex = new RegExp(CODE_BLOCK_REGEX.source, CODE_BLOCK_REGEX.flags);
			let match;
			while ((match = regex.exec(markdown)) !== null) {
				const rawContent = match[1];
				const parsed = tolerantJsonParse(rawContent);
				if (parsed) {
					const doc = normalizeLiveSpec(parsed);
					if (doc) specs.push(doc);
				}
			}
			return specs;
		}
		/**
		* Extracts a LiveSpecDoc from Markdown content.
		* Defaults to the last (latest revision) valid spec block if multiple exist.
		*/
		function extractSpecFromMarkdown(markdown, options) {
			const specs = extractAllSpecsFromMarkdown(markdown);
			if (specs.length === 0) return null;
			if (options?.pick === "first") return specs[0];
			return specs[specs.length - 1];
		}
		//#endregion
		//#region lib/src/client.js
		const LIVE_SPEC_TAB_KEY = "live-spec";
		const DEFAULT_CANVAS_URL = "/agentflow/canvas/index.html";
		/**
		* Live-Spec Tab Title Component registered in DSH right sidebar.
		*/
		function LiveSpecPaneTitle() {
			return react.default.createElement("div", {
				style: {
					display: "inline-flex",
					alignItems: "center",
					gap: "6px",
					fontSize: "13px",
					fontWeight: 500,
					userSelect: "none"
				},
				title: "Agentflow Live-Spec Interactive Canvas"
			}, react.default.createElement("svg", {
				width: 14,
				height: 14,
				viewBox: "0 0 24 24",
				fill: "none",
				stroke: "currentColor",
				strokeWidth: 2,
				strokeLinecap: "round",
				strokeLinejoin: "round"
			}, react.default.createElement("circle", {
				cx: 6,
				cy: 6,
				r: 3
			}), react.default.createElement("circle", {
				cx: 6,
				cy: 18,
				r: 3
			}), react.default.createElement("circle", {
				cx: 18,
				cy: 12,
				r: 3
			}), react.default.createElement("path", { d: "M9 6h4a2 2 0 0 1 2 2v4m0 0a2 2 0 0 1-2 2H9" })), react.default.createElement("span", null, "Live Spec"));
		}
		/**
		* Host Card Container with Embedded Iframe and Fullscreen Modal Toggle
		*/
		function LiveSpecHostCard({ canvasUrl = DEFAULT_CANVAS_URL, initialSpec = null, initialMeta = null, sessionContext: propSessionContext = null, sessionId: directSessionId, cwd: directCwd, useSessions, readOnly = false, theme = "dark", onApply, onFeedbackIntent, className }) {
			const [isFullscreen, setIsFullscreen] = (0, react.useState)(false);
			const [currentSpec, setCurrentSpec] = (0, react.useState)(initialSpec);
			const [isReady, setIsReady] = (0, react.useState)(false);
			const [lastNotification, setLastNotification] = (0, react.useState)(null);
			const resolvedSessionId = initialMeta?.sessionId ?? propSessionContext?.sessionId ?? directSessionId;
			const sessionContext = {
				sessionId: resolvedSessionId,
				cwd: typeof useSessions === "function" && resolvedSessionId ? useSessions((sessions) => sessions?.byId?.[resolvedSessionId]?.cwd) : initialMeta?.cwd ?? propSessionContext?.cwd ?? directCwd,
				...initialMeta,
				...propSessionContext
			};
			const sessionId = sessionContext.sessionId;
			const cwd = sessionContext.cwd;
			const iframeRef = (0, react.useRef)(null);
			const postToCanvas = (0, react.useCallback)((message) => {
				if (iframeRef.current?.contentWindow) iframeRef.current.contentWindow.postMessage(message, "*");
			}, []);
			(0, react.useEffect)(() => {
				if (initialSpec) {
					setCurrentSpec(initialSpec);
					if (isReady && iframeRef.current?.contentWindow) {
						const msg = {
							type: "SPEC_MOUNT",
							payload: {
								spec: initialSpec,
								readOnly,
								sessionContext: {
									sessionId,
									cwd
								}
							}
						};
						iframeRef.current.contentWindow.postMessage(msg, "*");
					}
				}
			}, [
				initialSpec,
				isReady,
				readOnly,
				sessionId,
				cwd
			]);
			(0, react.useEffect)(() => {
				if (isReady && iframeRef.current?.contentWindow) {
					const msg = {
						type: "SESSION_CONTEXT_CHANGE",
						payload: { sessionContext: {
							sessionId,
							cwd
						} }
					};
					iframeRef.current.contentWindow.postMessage(msg, "*");
				}
			}, [
				sessionId,
				cwd,
				isReady
			]);
			(0, react.useEffect)(() => {
				if (initialSpec) return;
				if (!cwd) {
					setCurrentSpec(null);
					return;
				}
				let isMounted = true;
				const fetchProjectStatus = async () => {
					try {
						const res = await fetch(`/api/agentflow/status?cwd=${encodeURIComponent(cwd)}`);
						if (res.ok) {
							const data = await res.json();
							if (isMounted && data) {
								const doc = data.spec ? normalizeLiveSpec(data.spec) : normalizeLiveSpec(data);
								if (doc && doc.tasks && doc.tasks.length > 0) setCurrentSpec(doc);
							}
						}
					} catch {}
				};
				fetchProjectStatus();
				return () => {
					isMounted = false;
				};
			}, [cwd, initialSpec]);
			(0, react.useEffect)(() => {
				const handleSpecBroadcast = (event) => {
					const customEvent = event;
					if (customEvent.detail) {
						if (customEvent.detail.cwd && cwd && customEvent.detail.cwd !== cwd) return;
						if (customEvent.detail.spec) {
							const doc = normalizeLiveSpec(customEvent.detail.spec);
							if (doc && doc.tasks && doc.tasks.length > 0) setCurrentSpec(doc);
						} else if (customEvent.detail.markdown) {
							const doc = extractSpecFromMarkdown(customEvent.detail.markdown);
							if (doc && doc.tasks && doc.tasks.length > 0) setCurrentSpec(doc);
						}
					}
				};
				window.addEventListener("agentflow:spec", handleSpecBroadcast);
				return () => {
					window.removeEventListener("agentflow:spec", handleSpecBroadcast);
				};
			}, [cwd]);
			(0, react.useEffect)(() => {
				const handleWindowMessage = (event) => {
					const data = event.data;
					if (!data || typeof data !== "object" || !data.type) return;
					switch (data.type) {
						case "CANVAS_READY":
							setIsReady(true);
							if (currentSpec) postToCanvas({
								type: "SPEC_MOUNT",
								payload: {
									spec: currentSpec,
									readOnly,
									sessionContext: {
										sessionId,
										cwd
									}
								}
							});
							else postToCanvas({
								type: "SESSION_CONTEXT_CHANGE",
								payload: { sessionContext: {
									sessionId,
									cwd
								} }
							});
							postToCanvas({
								type: "THEME_CHANGE",
								payload: { theme }
							});
							break;
						case "SPEC_APPLY": {
							const { spec, diff } = data.payload;
							setCurrentSpec(spec);
							setLastNotification(`Spec applied: ${diff?.summary || `${spec.tasks.length} tasks`}`);
							onApply?.({
								spec,
								diff
							});
							break;
						}
						case "SPEC_FEEDBACK_INTENT": {
							const { diff, spec, prompt } = data.payload;
							setLastNotification("Feedback sent to AI conversation");
							onFeedbackIntent?.({
								diff,
								spec,
								prompt
							});
							break;
						}
						case "REQUEST_FULLSCREEN": setIsFullscreen(Boolean(data.payload.fullscreen));
					}
				};
				window.addEventListener("message", handleWindowMessage);
				return () => {
					window.removeEventListener("message", handleWindowMessage);
				};
			}, [
				currentSpec,
				onApply,
				onFeedbackIntent,
				postToCanvas,
				readOnly,
				sessionId,
				cwd,
				theme
			]);
			const toggleFullscreen = (0, react.useCallback)(() => {
				setIsFullscreen((prev) => !prev);
			}, []);
			const containerStyle = isFullscreen ? {
				position: "fixed",
				inset: 0,
				zIndex: 99999,
				backgroundColor: theme === "light" ? "#f8fafc" : "#0f172a",
				display: "flex",
				flexDirection: "column",
				width: "100vw",
				height: "100vh",
				overflow: "hidden"
			} : {
				position: "relative",
				display: "flex",
				flexDirection: "column",
				width: "100%",
				height: "100%",
				minHeight: "480px",
				backgroundColor: theme === "light" ? "#ffffff" : "#090d16",
				borderRadius: "6px",
				overflow: "hidden"
			};
			const headerStyle = {
				display: "flex",
				alignItems: "center",
				justifyContent: "space-between",
				flexWrap: "wrap",
				gap: "8px",
				rowGap: "6px",
				minWidth: 0,
				padding: "8px 12px",
				backgroundColor: theme === "light" ? "#f1f5f9" : "#1e293b",
				borderBottom: theme === "light" ? "1px solid #e2e8f0" : "1px solid #334155",
				color: theme === "light" ? "#0f172a" : "#f8fafc",
				fontSize: "12px",
				lineHeight: 1.4
			};
			const buttonStyle = {
				display: "inline-flex",
				alignItems: "center",
				gap: "4px",
				padding: "4px 8px",
				fontSize: "12px",
				fontWeight: 500,
				borderRadius: "4px",
				border: theme === "light" ? "1px solid #cbd5e1" : "1px solid #475569",
				backgroundColor: theme === "light" ? "#ffffff" : "#334155",
				color: theme === "light" ? "#1e293b" : "#f8fafc",
				cursor: "pointer",
				transition: "background-color 0.15s ease",
				flexShrink: 0,
				whiteSpace: "nowrap"
			};
			Boolean(currentSpec && Array.isArray(currentSpec.tasks) && currentSpec.tasks.length > 0);
			return react.default.createElement("div", {
				className,
				style: containerStyle,
				"data-testid": "live-spec-host-container"
			}, react.default.createElement("div", { style: headerStyle }, react.default.createElement("div", { style: {
				display: "flex",
				alignItems: "center",
				gap: "8px",
				minWidth: 0,
				flex: "1 1 0%",
				overflow: "hidden"
			} }, react.default.createElement("span", { style: {
				fontWeight: 600,
				minWidth: 0,
				flexShrink: 1,
				overflow: "hidden",
				textOverflow: "ellipsis",
				whiteSpace: "nowrap"
			} }, currentSpec?.title || (cwd ? `工作区: ${cwd}` : "Agentflow Live-Spec Canvas")), currentSpec?.dag_id ? react.default.createElement("span", { style: {
				fontSize: "11px",
				padding: "2px 6px",
				borderRadius: "4px",
				backgroundColor: theme === "light" ? "#e2e8f0" : "#334155",
				color: theme === "light" ? "#475569" : "#94a3b8",
				maxWidth: "240px",
				minWidth: 0,
				flexShrink: 1,
				overflow: "hidden",
				textOverflow: "ellipsis",
				whiteSpace: "nowrap"
			} }, currentSpec.dag_id) : cwd ? react.default.createElement("span", {
				"data-testid": "header-cwd-badge",
				style: {
					fontSize: "11px",
					padding: "2px 6px",
					borderRadius: "4px",
					backgroundColor: theme === "light" ? "#e2e8f0" : "#334155",
					color: theme === "light" ? "#475569" : "#94a3b8",
					maxWidth: "240px",
					minWidth: 0,
					flexShrink: 1,
					overflow: "hidden",
					textOverflow: "ellipsis",
					whiteSpace: "nowrap"
				},
				title: cwd
			}, cwd) : null, lastNotification ? react.default.createElement("span", { style: {
				fontSize: "11px",
				color: "#10b981",
				marginLeft: "8px",
				minWidth: 0,
				flexShrink: 1,
				overflow: "hidden",
				textOverflow: "ellipsis",
				whiteSpace: "nowrap"
			} }, `✓ ${lastNotification}`) : null), react.default.createElement("div", { style: {
				display: "flex",
				alignItems: "center",
				gap: "8px",
				flexShrink: 0
			} }, react.default.createElement("button", {
				type: "button",
				onClick: toggleFullscreen,
				style: buttonStyle,
				title: isFullscreen ? "收起全屏 (Esc)" : "展开全屏模式",
				"data-testid": "fullscreen-toggle-btn"
			}, isFullscreen ? "⤡ 收起" : "⤢ 展开"))), react.default.createElement("iframe", {
				ref: iframeRef,
				src: canvasUrl,
				title: "Agentflow Live Spec Canvas",
				allow: "clipboard-write; fullscreen",
				style: {
					flex: 1,
					width: "100%",
					height: "100%",
					border: "none",
					backgroundColor: "transparent"
				}
			}));
		}
		/**
		* Live-Spec Pane Body Component registered to DSH slot: sidebar.right.pane.tab
		*/
		function LiveSpecPaneBody(props) {
			const sessionId = props?.sessionId ?? props?.initialMeta?.sessionId;
			const useSessions = props?.useSessions;
			const cwd = typeof useSessions === "function" ? useSessions((sessions) => sessions?.byId?.[sessionId]?.cwd) : props?.cwd ?? props?.initialMeta?.cwd;
			const initialMeta = {
				...props?.initialMeta || {},
				sessionId,
				cwd
			};
			return react.default.createElement(LiveSpecHostCard, {
				canvasUrl: props?.canvasUrl || "/agentflow/canvas/index.html",
				initialSpec: props?.spec ?? props?.initialSpec ?? null,
				initialMeta,
				sessionContext: {
					sessionId,
					cwd
				},
				readOnly: props?.readOnly ?? false,
				theme: props?.theme || "dark",
				onApply: props?.onApply,
				onFeedbackIntent: props?.onFeedbackIntent
			});
		}
		const name = "dsh-interactive-spec";
		const inject = ["slots", "sidebarRightTabs"];
		/**
		* Client plugin entry point for DeepSeek Harness (Cordis runner).
		*/
		function apply(ctx) {
			const registerSlots = () => {
				if (ctx.sidebarRightTabs && typeof ctx.sidebarRightTabs.register === "function") ctx.sidebarRightTabs.register({
					id: LIVE_SPEC_TAB_KEY,
					kind: LIVE_SPEC_TAB_KEY,
					title: () => "Live Spec",
					guide: [{
						order: 15,
						title: () => "Live Spec Canvas",
						description: () => "Interactive DAG canvas with real-time simulation and CPM analysis"
					}]
				});
				const unregisterTitle = ctx.slots.inject("sidebar.right.pane.tab.title", () => ctx.slots.register({
					name: "sidebar.right.pane.tab.title",
					key: LIVE_SPEC_TAB_KEY
				}, LiveSpecPaneTitle));
				const unregisterBody = ctx.slots.inject("sidebar.right.pane.tab", () => ctx.slots.register({
					name: "sidebar.right.pane.tab",
					key: LIVE_SPEC_TAB_KEY
				}, LiveSpecPaneBody));
				return () => {
					unregisterTitle?.();
					unregisterBody?.();
				};
			};
			if (typeof ctx.effect === "function") ctx.effect(registerSlots, "dsh-interactive-spec: sidebar slots");
			else registerSlots();
		}
		//#endregion
		exports.DEFAULT_CANVAS_URL = DEFAULT_CANVAS_URL;
		exports.LIVE_SPEC_TAB_KEY = LIVE_SPEC_TAB_KEY;
		exports.LiveSpecHostCard = LiveSpecHostCard;
		exports.LiveSpecPaneBody = LiveSpecPaneBody;
		exports.LiveSpecPaneTitle = LiveSpecPaneTitle;
		exports.apply = apply;
		exports.inject = inject;
		exports.name = name;
		return module.exports;
	}
});
