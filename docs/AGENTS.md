# AGENTS.md

## Inicio y fuentes de verdad

Antes de una tarea nueva, leer [CONTEXT](CONTEXT.md) y [MEMORY](MEMORY.md).
Consultar solo las secciones pertinentes de [PLAN](PLAN.md); no cargar todo
el plan para un cambio pequeño.

- `AGENTS.md`: instrucciones para trabajar y verificar.
- `CONTEXT.md`: propósito, términos, decisiones estables y límites del producto.
- `PLAN.md`: contratos detallados, fases y criterios de aceptación.
- `COMPATIBILITY.md`: contrato de fase 0 contrastado con fuentes fijadas; consultar
  para precedencia, rutas, errores y cambios intencionales frente al upstream.
- `MEMORY.md`: estado comprobado, pendientes y siguiente paso.

Las instrucciones actuales del usuario prevalecen. Verificar el checkout antes
de asumir que una propuesta está implementada. Si código, contrato y memoria
difieren, señalar la diferencia y resolverla dentro del alcance solicitado.

## Ejecución

- Investigar archivos, llamadas y pruebas relevantes antes de editar. Usar
  búsquedas dirigidas; ampliar la lectura solo cuando sea necesario.
- Hacer el menor cambio correcto. Reutilizar lo existente y la biblioteca estándar.
  Evitar refactors ajenos, paquetes vacíos, dependencias y tooling preventivos.
- Separar respuestas de UI, construcción/validación y efectos sobre Git o archivos.
  Mantener un constructor compartido para cliente, hook y ejecución no interactiva.
- Implementar por incrementos verificables de la fase activa. Resolver las dudas
  que afectan el incremento antes de fijar comportamientos; continuar el trabajo
  independiente de esas dudas sin pedir aprobaciones rutinarias.
- Conservar los cambios del usuario y los avisos de licencia. No modificar ni
  distribuir `.agents/`, `.codex/` o `.aws/`.
- No efectuar commits, push, publicaciones, retiradas destructivas o cambios de
  configuración global sin autorización. Los repositorios temporales de pruebas
  pueden tener commits e identidad local.
- Cuando se soliciten commits de desarrollo, usar los seis tipos del contrato,
  cambios lógicos pequeños y alcances reales; conservar el historial previo.
- En este entorno, prefijar comandos de shell con `rtk`; usar `rtk proxy` cuando
  se necesite salida íntegra o un comando sin soporte específico.

## Uso de agentes y modelos

Trabajar directamente en el agente principal por defecto. Delegar únicamente
tareas sustanciales independientes o investigaciones que requieran contexto
aislado; no delegar búsquedas, lecturas breves, ediciones simples o checks.

Al delegar, indicar objetivo, archivos asignados, contrato aplicable y resultado
esperado. Evitar ediciones concurrentes de los mismos archivos; el agente principal
integra el resultado y verifica su coherencia.

La recomendación Luna 6 / Sol 6.1 está en PLAN, sección 10.1. Respetar el modelo
elegido por el usuario. En un relevo, entregar archivos afectados, decisiones,
checks y dudas pendientes; usar MEMORY como estado común.

## Protección de datos

- Git se ejecuta con argumentos separados; los mensajes nunca se interpolan en shell.
- Respetar staging, firma, hooks e identidad. Alterar staging solo por elección explícita.
- Instalar, actualizar o retirar exclusivamente hooks reconocidos como propios.
- Validar antes de reemplazar mensajes o caché; conservar originales ante fallo.
- Distinguir error, cancelación y ausencia de TTY según el contrato del plan.
- Verificar datos y tipos en las fronteras: argumentos, JSON, archivos y HTTP.
- Recuperar recursos upstream por archivo y commit fijado, según PLAN 9.1.
  No reclonar, descargar ZIPs completos ni introducir Node para recuperarlos.

## Verificación y cierre

Con fuentes Go disponibles, formatear los archivos modificados con `gofmt -w`
y ejecutar los checks del proyecto:

```sh
go vet ./...
go test ./...
go build -o /tmp/gitmoji ./cmd/gitmoji
```

Usar pruebas de comportamiento para lógica nueva y regresiones, con `testing`,
`httptest` y repositorios temporales. Elegir los casos afectados de PLAN 11;
no trasladar cada mock o snapshot heredado ni repetir checks sin motivo.
La compilación cruzada no acredita terminal ni hooks en la plataforma destino.

Para documentación, comprobar enlaces locales, coherencia y diff; no ejecutar
checks de código cuando no haya cambios de implementación.

Actualizar MEMORY con estado observado, decisiones pendientes, resultado de checks
y siguiente paso. Actualizar CONTEXT o PLAN solo si cambia su contrato; evitar
duplicar requisitos o mantener diarios extensos. No marcar una fase completa
hasta cumplir sus criterios de salida. Entregar cambios, verificación y
limitaciones reales de forma breve.

Registrar trabajo sustancial y fuentes/checks en `docs/agent-logs/`, con nombre
`AAAA-MM-DD-fase-N-tarea.md`. Los logs son locales; no guardar secretos ni dumps
innecesarios. El estado actual para retomar sigue siendo MEMORY.
