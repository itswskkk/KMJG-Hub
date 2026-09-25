import { useEffect, useState } from "react";
import {
  DeveloperTool,
  GitRepository,
  currentGitRepository,
  isTauri,
  isToolInstalled,
  launchTerminal,
  launchTool,
  launchVSCode,
} from "../../lib/nativeClient";
import "./DeveloperTools.css";

interface DeveloperToolsProps {
  projectId: string;
}

interface ToolDef {
  key: string;
  label: string;
  installKey: string;
  launch: (projectId: string) => Promise<void>;
}

const TOOLS: ToolDef[] = [
  { key: "terminal", label: "Terminal", installKey: "terminal", launch: launchTerminal },
  { key: "vscode", label: "VS Code", installKey: "code", launch: launchVSCode },
  { key: "codex", label: "Codex", installKey: "codex", launch: (projectId) => launchTool(projectId, "codex" as DeveloperTool) },
  { key: "claude", label: "Claude Code", installKey: "claude", launch: (projectId) => launchTool(projectId, "claude" as DeveloperTool) },
  { key: "opencode", label: "OpenCode", installKey: "opencode", launch: (projectId) => launchTool(projectId, "opencode" as DeveloperTool) },
];

/**
 * Developer Tools, per docs/PRD.md "Developer Tool Integration" and
 * docs/VISION.md "Developer Tools": KMJG Hub launches external tools at the
 * Project's local directory rather than replacing them. That directory
 * comes from the same "Select Repository" flow used for Work Status
 * (features/work-context), so this section is only useful once that has
 * been set; otherwise it points there instead of showing five buttons that
 * would all fail.
 */
function DeveloperTools({ projectId }: DeveloperToolsProps) {
  const [repo, setRepo] = useState<GitRepository | null | undefined>(undefined);
  const [installed, setInstalled] = useState<Record<string, boolean | undefined>>({});
  const [pending, setPending] = useState<string | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    let cancelled = false;
    setRepo(undefined);
    if (!isTauri()) {
      setRepo(null);
      return;
    }
    currentGitRepository(projectId)
      .then((loaded) => {
        if (!cancelled) setRepo(loaded);
      })
      .catch(() => {
        if (!cancelled) setRepo(null);
      });
    return () => {
      cancelled = true;
    };
  }, [projectId]);

  useEffect(() => {
    if (!isTauri()) return;
    let cancelled = false;
    TOOLS.forEach((tool) => {
      isToolInstalled(tool.installKey)
        .then((ok) => {
          if (!cancelled) setInstalled((current) => ({ ...current, [tool.key]: ok }));
        })
        .catch(() => {
          if (!cancelled) setInstalled((current) => ({ ...current, [tool.key]: false }));
        });
    });
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleLaunch(tool: ToolDef) {
    setErrors((current) => ({ ...current, [tool.key]: "" }));
    setPending(tool.key);
    try {
      await tool.launch(projectId);
    } catch (err) {
      setErrors((current) => ({
        ...current,
        [tool.key]: err instanceof Error ? err.message : `Could not open ${tool.label}.`,
      }));
    } finally {
      setPending(null);
    }
  }

  if (!isTauri()) {
    return (
      <section className="developer-tools">
        <h2>Developer Tools</h2>
        <p className="developer-tools__note">Developer Tools are available in the desktop app.</p>
      </section>
    );
  }

  if (repo === undefined) {
    return (
      <section className="developer-tools">
        <h2>Developer Tools</h2>
        <p className="developer-tools__note">Loading...</p>
      </section>
    );
  }

  if (!repo) {
    return (
      <section className="developer-tools">
        <h2>Developer Tools</h2>
        <p className="developer-tools__note">
          No local directory is configured for this Project yet. Use "Select Repository" in the Work Status bar
          above to choose one, then Developer Tools can open it directly.
        </p>
      </section>
    );
  }

  return (
    <section className="developer-tools">
      <h2>Developer Tools</h2>
      <p className="developer-tools__path" title={repo.path}>
        {repo.path}
      </p>
      <div className="developer-tools__grid">
        {TOOLS.map((tool) => {
          const toolInstalled = installed[tool.key];
          const disabled = pending !== null || toolInstalled === false;
          return (
            <div className="developer-tools__tool" key={tool.key}>
              <button type="button" onClick={() => void handleLaunch(tool)} disabled={disabled}>
                {pending === tool.key ? "Opening..." : tool.label}
              </button>
              {toolInstalled === false && <span className="developer-tools__badge">Not Installed</span>}
              {errors[tool.key] && (
                <p className="developer-tools__error" role="alert">
                  {errors[tool.key]}
                </p>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}

export default DeveloperTools;
