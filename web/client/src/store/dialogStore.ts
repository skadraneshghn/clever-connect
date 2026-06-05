import { create } from 'zustand';

export type DialogType = 'alert' | 'confirm' | 'prompt';
export type DialogVariant = 'success' | 'error' | 'warning' | 'info' | 'question';

export interface DialogConfig {
  type: DialogType;
  variant: DialogVariant;
  title: string;
  message: string;
  defaultValue?: string;
  placeholder?: string;
  resolve: (value: any) => void;
}

interface DialogState {
  currentDialog: DialogConfig | null;
  alert: (message: string, options?: { title?: string; variant?: DialogVariant }) => Promise<void>;
  confirm: (message: string, options?: { title?: string; variant?: DialogVariant }) => Promise<boolean>;
  prompt: (message: string, defaultValue?: string, options?: { title?: string; placeholder?: string }) => Promise<string | null>;
  close: (value: any) => void;
}

export const useDialogStore = create<DialogState>((set, get) => ({
  currentDialog: null,

  alert: (message, options) => {
    return new Promise<void>((resolve) => {
      set({
        currentDialog: {
          type: 'alert',
          variant: options?.variant || 'info',
          title: options?.title || 'Alert',
          message,
          resolve: () => {
            set({ currentDialog: null });
            resolve();
          },
        },
      });
    });
  },

  confirm: (message, options) => {
    return new Promise<boolean>((resolve) => {
      set({
        currentDialog: {
          type: 'confirm',
          variant: options?.variant || 'question',
          title: options?.title || 'Confirmation',
          message,
          resolve: (result: boolean) => {
            set({ currentDialog: null });
            resolve(result);
          },
        },
      });
    });
  },

  prompt: (message, defaultValue = '', options) => {
    return new Promise<string | null>((resolve) => {
      set({
        currentDialog: {
          type: 'prompt',
          variant: 'question',
          title: options?.title || 'Prompt',
          message,
          defaultValue,
          placeholder: options?.placeholder || 'Type here...',
          resolve: (result: string | null) => {
            set({ currentDialog: null });
            resolve(result);
          },
        },
      });
    });
  },

  close: (value) => {
    const dialog = get().currentDialog;
    if (dialog) {
      dialog.resolve(value);
    }
  },
}));

export const showGlobalAlert = (message: string, options?: { title?: string; variant?: DialogVariant }) =>
  useDialogStore.getState().alert(message, options);

export const showGlobalConfirm = (message: string, options?: { title?: string; variant?: DialogVariant }) =>
  useDialogStore.getState().confirm(message, options);

export const showGlobalPrompt = (message: string, defaultValue?: string, options?: { title?: string; placeholder?: string }) =>
  useDialogStore.getState().prompt(message, defaultValue, options);
