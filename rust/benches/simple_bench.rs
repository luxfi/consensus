use criterion::{criterion_group, criterion_main, Criterion};

fn simple_bench(c: &mut Criterion) {
    c.bench_function("simple", |b| b.iter(|| 2 + 2));
}

criterion_group!(benches, simple_bench);
criterion_main!(benches);