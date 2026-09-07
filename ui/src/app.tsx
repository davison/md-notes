import { LocationProvider, Router, Route } from "preact-iso";
import { Home } from "./home";
import { RootView } from "./root-view";

function NotFound() {
  return (
    <main class="page">
      <h1>Not found</h1>
      <p>
        <a href="/">Back to roots</a>
      </p>
    </main>
  );
}

export function App() {
  return (
    <LocationProvider>
      <Router>
        <Route path="/" component={Home} />
        <Route path="/r/:slug/*" component={RootView} />
        <Route path="/r/:slug" component={RootView} />
        <Route default component={NotFound} />
      </Router>
    </LocationProvider>
  );
}
