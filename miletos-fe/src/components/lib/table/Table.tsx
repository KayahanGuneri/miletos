import type { ComponentPropsWithoutRef } from "react";

export const Table = (props: ComponentPropsWithoutRef<"table">) => <table {...props} />;

export const TableHeader = (props: ComponentPropsWithoutRef<"thead">) => <thead {...props} />;

export const TableBody = (props: ComponentPropsWithoutRef<"tbody">) => <tbody {...props} />;

export const TableRow = (props: ComponentPropsWithoutRef<"tr">) => <tr {...props} />;

export const TableHead = (props: ComponentPropsWithoutRef<"th">) => <th {...props} />;

export const TableCell = (props: ComponentPropsWithoutRef<"td">) => <td {...props} />;
