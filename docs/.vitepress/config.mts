import { defineConfig } from "vitepress";

const hostname = process.env.DOCS_HOSTNAME;

export default defineConfig({
  lang: "en-US",
  base: process.env.DOCS_BASE || "/",
  title: "glim",
  description: "Publish HTML previews and get a short shareable link — CLI + MCP",
  ...(hostname ? { sitemap: { hostname } } : {}),
  cleanUrls: true,
  appearance: false,
  themeConfig: {
    logo: { src: "/icon.svg", alt: "glim" },
    nav: [
      { text: "Guide", link: "/getting-started" },
      { text: "CLI", link: "/cli" },
      { text: "Agents", link: "/agents" },
    ],
    sidebar: [
      {
        text: "Guide",
        items: [
          { text: "Getting started", link: "/getting-started" },
          { text: "CLI reference", link: "/cli" },
          { text: "Configuration", link: "/config" },
          { text: "Serving & reverse proxy", link: "/serving" },
          { text: "Agent integration (MCP)", link: "/agents" },
        ],
      },
    ],
    socialLinks: [{ icon: "github", link: "https://github.com/samuelloranger/glim" }],
    outline: [2, 3],
  },
});
