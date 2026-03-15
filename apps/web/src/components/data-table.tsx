"use client";

import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

export interface DataTableColumn<T> {
  id: string;
  header: string;
  width?: string;
  headerClassName?: string;
  cellClassName?: string;
  cell: (row: T) => React.ReactNode;
}

interface DataTableProps<T> {
  columns: DataTableColumn<T>[];
  data: T[];
  keyExtractor: (row: T) => string;
  mobileCard: (row: T) => React.ReactNode;
  minWidth?: number;
  rowClassName?: string;
}

export function DataTable<T>({
  columns,
  data,
  keyExtractor,
  mobileCard,
  minWidth = 900,
  rowClassName,
}: DataTableProps<T>) {
  return (
    <>
      <div className="flex flex-col gap-3 md:hidden">
        {data.map((row) => (
          <div key={keyExtractor(row)}>{mobileCard(row)}</div>
        ))}
      </div>

      <div className="hidden md:block">
        <Table style={{ minWidth }}>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              {columns.map((col) => (
                <TableHead
                  key={col.id}
                  className={cn(
                    "h-auto px-4 py-2.5 text-[11px] font-semibold uppercase tracking-wider text-dim",
                    col.headerClassName,
                  )}
                  style={col.width ? { width: col.width } : undefined}
                >
                  {col.header}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.map((row) => (
              <TableRow key={keyExtractor(row)} className={rowClassName}>
                {columns.map((col) => (
                  <TableCell
                    key={col.id}
                    className={cn("px-4 py-3", col.cellClassName)}
                  >
                    {col.cell(row)}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </>
  );
}
