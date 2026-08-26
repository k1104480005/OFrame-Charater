// Warm-white / dark-ink theme toggle (workbench-ui spec 10.8).
import { useTheme } from "../theme/ThemeProvider";
import "./ThemeToggle.css";

export function ThemeToggle() {
  const { theme, toggleTheme } = useTheme();
  const dark = theme === "dark-ink";
  return (
    <button
      type="button"
      className={`pixel-btn theme-toggle${dark ? " theme-toggle--dark" : ""}`}
      onClick={toggleTheme}
      aria-label={dark ? "切换到暖白主题" : "切换到深墨主题"}
      title={dark ? "切换到暖白主题" : "切换到深墨主题"}
    >
      <span className="theme-toggle__icon" aria-hidden="true">
        {dark ? "◐" : "●"}
      </span>
      <span className="mono">{dark ? "DARK" : "WARM"}</span>
    </button>
  );
}
