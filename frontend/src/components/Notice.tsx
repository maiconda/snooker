type NoticeProps = {
  message: string | null;
};

export function Notice({ message }: NoticeProps) {
  if (!message) {
    return null;
  }

  return <p className="border border-neutral-300 bg-neutral-50 px-3 py-2 text-sm text-neutral-800">{message}</p>;
}


