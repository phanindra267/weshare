import { cn } from "@/lib/cn";
import { Button } from "@/components/ui/Button";

export interface ErrorMessageProps {
  title?: string;
  message: string;
  onRetry?: () => void;
  className?: string;
}

/** Inline, non-decorative error state with an optional retry action. */
export function ErrorMessage({
  title = "Something went wrong",
  message,
  onRetry,
  className,
}: ErrorMessageProps) {
  return (
    <div
      role="alert"
      className={cn(
        "rounded-lg border border-red-300 bg-red-50 p-4 text-sm dark:border-red-900 dark:bg-red-950/40",
        className,
      )}
    >
      <p className="font-medium text-red-800 dark:text-red-200">{title}</p>
      <p className="mt-1 text-red-700 dark:text-red-300">{message}</p>
      {onRetry ? (
        <Button
          variant="secondary"
          size="sm"
          className="mt-3"
          onClick={onRetry}
        >
          Try again
        </Button>
      ) : null}
    </div>
  );
}
