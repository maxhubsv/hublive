interface SettingSectionProps {
  title: string;
  children: React.ReactNode;
}

export function SettingSection({ title, children }: SettingSectionProps) {
  return (
    <div>
      <h2 className="mb-element text-caption font-semibold uppercase tracking-wider text-text-secondary">
        {title}
      </h2>
      <div className="overflow-hidden rounded-lg border border-bg-tertiary bg-bg-secondary">
        {children}
      </div>
    </div>
  );
}
