import { Switch, Route, Router as WouterRouter } from "wouter";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "@/components/ui/toaster";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AppShell } from "@/components/layout/Shell";
import Dashboard from "@/pages/Dashboard";
import Tasks from "@/pages/Tasks";
import Logs from "@/pages/Logs";
import NotFound from "@/pages/not-found";
import LoginPage from "@/pages/Login";
import { AuthProvider, useAuth } from "@/context/auth";

function makeQueryClient(onUnauthorized: () => void) {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: (failureCount, error) => {
          if (
            error &&
            typeof error === "object" &&
            "status" in error &&
            (error as { status: number }).status === 401
          ) {
            onUnauthorized();
            return false;
          }
          return failureCount < 2;
        },
      },
    },
  });
}

function AuthGate() {
  const { status, markUnauthenticated } = useAuth();

  const queryClient = makeQueryClient(markUnauthenticated);

  if (status === "loading") {
    return (
      <div className="min-h-screen bg-[#0d1117] flex items-center justify-center">
        <span className="text-xs font-mono text-gray-500">Connecting...</span>
      </div>
    );
  }

  if (status === "unauthenticated") {
    return <LoginPage />;
  }

  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <WouterRouter base={import.meta.env.BASE_URL.replace(/\/$/, "")}>
          <AppShell>
            <Switch>
              <Route path="/" component={Dashboard} />
              <Route path="/tasks" component={Tasks} />
              <Route path="/logs" component={Logs} />
              <Route component={NotFound} />
            </Switch>
          </AppShell>
        </WouterRouter>
        <Toaster />
      </TooltipProvider>
    </QueryClientProvider>
  );
}

function App() {
  return (
    <AuthProvider>
      <AuthGate />
    </AuthProvider>
  );
}

export default App;
