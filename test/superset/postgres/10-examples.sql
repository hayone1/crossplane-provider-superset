-- A small table that the examples build datasets, charts, dashboards and alerts on.
CREATE TABLE public.sales (
    id integer PRIMARY KEY,
    region text NOT NULL,
    amount numeric(10, 2) NOT NULL,
    day date NOT NULL
);

INSERT INTO public.sales (id, region, amount, day) VALUES
    (1, 'north', 10.50, '2026-01-01'),
    (2, 'south', 20.00, '2026-01-02'),
    (3, 'north', 7.25, '2026-01-03');
