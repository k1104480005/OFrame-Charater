// Root component: theme + session providers, then route between the launch
// page (no package open) and the three-tab main screen (package open). The
// session package instance lives on the Go side and is shared by every tab.
import { ThemeProvider } from "./theme/ThemeProvider";
import { SessionProvider, useSession } from "./state/SessionContext";
import { LaunchPage } from "./pages/LaunchPage";
import { MainScreen } from "./pages/MainScreen";

function Root() {
  const { pkg, loading, error } = useSession();
  if (loading) {
    return (
      <div className="empty-state" style={{ margin: "40vh auto", maxWidth: 480 }}>
        加载工作区…
      </div>
    );
  }
  if (error && !pkg) {
    return (
      <div className="empty-state" style={{ margin: "40vh auto", maxWidth: 480 }}>
        <div className="error-text">{error}</div>
      </div>
    );
  }
  return pkg ? <MainScreen /> : <LaunchPage />;
}

export default function App() {
  return (
    <ThemeProvider>
      <SessionProvider>
        <Root />
      </SessionProvider>
    </ThemeProvider>
  );
}
