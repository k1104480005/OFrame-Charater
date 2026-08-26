import React from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "@fontsource/press-start-2p"; // pixel font accents (10.8)
import "./styles/tokens.css";
import "./styles/global.css";

const container = document.getElementById("root");
if (!container) throw new Error("root element not found");

createRoot(container).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
