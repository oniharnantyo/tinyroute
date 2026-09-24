import React from 'react';
import { Modal } from './Modal';
import { Button } from './Button';

interface Props {
  open: boolean;
  title: string;
  description?: string;
  confirmLabel?: string;
  destructive?: boolean;
  onConfirm: () => void;
  onClose: () => void;
}

/** Confirmation dialog — the app-wide replacement for native confirm(). */
export const ConfirmDialog: React.FC<Props> = ({
  open,
  title,
  description,
  confirmLabel = 'Confirm',
  destructive = true,
  onConfirm,
  onClose,
}) => {
  return (
    <Modal isOpen={open} onClose={onClose} title={title} maxWidth="sm">
      {description && <p className="text-sm text-muted-foreground leading-relaxed">{description}</p>}
      <div className="flex justify-end gap-2.5 pt-2">
        <Button type="button" variant="outline" size="sm" onClick={onClose}>
          Cancel
        </Button>
        <Button
          type="button"
          size="sm"
          variant={destructive ? 'destructive' : 'primary'}
          onClick={() => {
            onConfirm();
            onClose();
          }}
        >
          {confirmLabel}
        </Button>
      </div>
    </Modal>
  );
};
