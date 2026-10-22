"use client"

import {Button} from "@/shared/ui/shadcn/button"
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/shared/ui/shadcn/dialog"

type ClearScheduleDialogProps = {
    open: boolean
    pending: boolean
    onClose: () => void
    onConfirm: () => void
}

export function ClearScheduleDialog({open, pending, onClose, onConfirm}: ClearScheduleDialogProps) {
    return (
        <Dialog onOpenChange={(nextOpen) => !nextOpen && !pending && onClose()} open={open}>
            <DialogContent className="sm:max-w-md" showCloseButton={!pending}>
                <DialogHeader>
                    <DialogTitle>Очистить всё расписание?</DialogTitle>
                    <DialogDescription>
                        Будут безвозвратно удалены все записи календаря, включая отменённые, планы пациентов
                        и отчёты по всем датам. Каталог процедур и настройки календаря сохранятся.
                    </DialogDescription>
                </DialogHeader>
                <DialogFooter>
                    <Button disabled={pending} onClick={onClose} type="button" variant="outline">Отмена</Button>
                    <Button disabled={pending} onClick={onConfirm} type="button" variant="destructive">
                        {pending ? "Очистка…" : "Удалить всё расписание"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    )
}
