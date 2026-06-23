type NoticeProps = {
  message: string | null;
};

export function Notice({ message }: NoticeProps) {
  if (!message) {
    return null;
  }

  const isSuccess = /sucesso|atualizado|salvo|concluído|pronto/i.test(message);

  if (isSuccess) {
    return (
      <p className="rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-2.5 text-sm text-red-600 dark:text-red-400 animate-fade-in">
        {message}
      </p>
    );
  }

  return (
    <p className="rounded-lg border border-rose-600/30 bg-rose-600/10 px-4 py-2.5 text-sm text-rose-700 dark:text-rose-400 animate-fade-in font-semibold">
      {message}
    </p>
  );
}



