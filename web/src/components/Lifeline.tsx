export function Lifeline(props: { fraction: number; pinned: boolean; ember: boolean }) {
  const value = () => (props.pinned ? 1 : props.fraction);
  return (
    // biome-ignore lint/a11y/useSemanticElements: Custom meter needs the styled inner span.
    <div
      class={["lifeline", { ember: props.ember, pinned: props.pinned }]}
      role="meter"
      aria-label="Time left"
      aria-valuemin="0"
      aria-valuemax="100"
      aria-valuenow={String(Math.round(value() * 100))}
    >
      <span style={{ transform: `scaleX(${value()})` }} />
    </div>
  );
}
