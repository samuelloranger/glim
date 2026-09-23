export function Logo(props: { size?: number; glance?: boolean }) {
  return (
    <svg
      class={["logo", { glance: props.glance === true }]}
      width={props.size ?? 28}
      height={props.size ?? 28}
      viewBox="0 0 64 64"
      fill="none"
      aria-hidden="true"
    >
      <rect width="64" height="64" rx="14" fill="#0f1526" />
      <path
        d="M32 20c-9 0-16.5 6-19 12 2.5 6 10 12 19 12s16.5-6 19-12c-2.5-6-10-12-19-12Z"
        stroke="#818cf8"
        stroke-width="3.5"
        fill="none"
      />
      <circle cx="32" cy="32" r="6.5" fill="#818cf8" />
      <circle class="logo-glint" cx="34" cy="30" r="2" fill="#e2e8f0" />
    </svg>
  );
}
