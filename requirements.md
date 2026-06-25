# VALORANT GRID
## Documento de Requerimientos y Backlog Inicial
**Juego tipo “tic-tac-toe” de conocimiento sobre Valorant esports**  
*Versión 0.1 — Borrador para revisión*  
*Junio 2026*

---

## 1. Visión del producto
Valorant Grid es un juego de adivinanzas tipo “tic-tac-toe” inspirado en los formatos de fútbol (ej. Footy TacToe). El jugador recibe un tablero de 3x3 donde cada fila y columna representa una categoría (equipo, región, rol, agente jugado, etc.). El objetivo es completar el tablero nombrando un jugador profesional de Valorant que cumpla simultáneamente con la categoría de la fila y la columna de cada celda.

A diferencia de los wordles o trivias existentes de Valorant (Valorantle, Valonerd, GuessTheAgent), no se identificó un competidor directo con formato de grid/intersección de categorías para Valorant. Esto representa una oportunidad de diferenciación.

### 1.1 Objetivos del proyecto
*   **Objetivo de portafolio:** Demostrar manejo de un stack completo (mobile, backend en Go, base de datos relacional) bajo una metodología de trabajo ágil simplificada, como evidencia para procesos de contratación.
*   **Objetivo de aprendizaje:** Ganar experiencia práctica con Go en un proyecto real (no tutorial), aplicando buenas prácticas de arquitectura backend.
*   **Objetivo de producto:** Lanzar un MVP jugable en Android (Play Store) con potencial de monetización futura vía publicidad, sin incurrir en costos de infraestructura.

### 1.2 Público objetivo
*   Jugadores y aficionados de Valorant con interés en la escena competitiva (esports).
*   Usuarios de juegos casuales de trivia/adivinanza (ya familiarizados con Wordle, Footy TacToe, Sporcle).

### 1.3 Restricciones del proyecto
*   **Presupuesto:** $0. Toda herramienta, servicio de hosting y base de datos debe operar dentro de un tier gratuito.
*   **Equipo:** 1 desarrollador principal, con colaboración ocasional de un segundo desarrollador.
*   **Plataforma inicial:** Android (Google Play Store). La arquitectura debe permitir migrar la capa de presentación a web (React) sin rehacer la lógica de negocio ni el backend.

---

## 2. Stack tecnológico y justificación

### 2.1 Frontend móvil — React Native + Expo
Se recomienda usar Expo (no React Native CLI “bare”) por su flujo de build administrado (EAS Build), que evita configurar Android Studio/Gradle manualmente y tiene un tier gratuito viable para apps pequeñas. La lógica de negocio (hooks, validación de respuestas, llamadas a la API) se debe escribir de forma agnóstica a la UI, de modo que al migrar a React web solo se reescriban los componentes visuales.

### 2.2 Backend — Go
Se recomienda iniciar con la librería estándar (`net/http`) combinada con un router ligero (`chi` o `gorilla/mux`), en lugar de un framework como Gin desde el principio. Esto maximiza el aprendizaje de los fundamentos del lenguaje. Para el acceso a datos se recomienda `pgx` o `sqlc` en lugar de un ORM pesado, ya que demuestra dominio de SQL directo, algo valorado en procesos de selección técnica.

**Estructura de carpetas sugerida (arquitectura en capas):**
*   `/cmd` — Punto de entrada de la aplicación.
*   `/internal/handler` — Controladores HTTP.
*   `/internal/service` — Lógica de negocio (generación de grid, validación).
*   `/internal/repository` — Acceso a base de datos.
*   `/internal/model` — Entidades del dominio.

### 2.3 Base de datos — PostgreSQL
Se recomienda PostgreSQL sobre MongoDB para este proyecto. La razón principal es que los datos son intrínsecamente relacionales: un jugador pertenece a un equipo, un equipo a una región, y un jugador participa en partidas con agentes específicos. Las consultas centrales del juego (*“jugadores que jugaron en el equipo X Y son de la región Y Y jugaron el agente Z”*) son intersecciones clásicas de conjuntos, el caso de uso ideal para SQL con `JOINs` e índices, y requerirían normalización manual o duplicación de datos en un modelo documental como MongoDB.

Como ventaja adicional para portafolio: modelar correctamente claves foráneas, índices compuestos para las consultas de intersección, y normalización, demuestra una competencia más transferible a entornos profesionales que el uso de NoSQL en un dominio que no lo requiere.

> **Hosting gratuito sugerido para Postgres:** Supabase o Neon (ambos con tier gratuito permanente, sin tarjeta de crédito asegurada en el momento de escribir esto — verificar vigencia de condiciones antes de decidir).

### 2.4 Hosting y distribución
*   **Repositorios:** GitHub (públicos, para portafolio).
*   **Backend:** Render o Fly.io (tier gratuito) — evaluar en el sprint de infraestructura.
*   **App móvil:** Google Play Store. *Nota: la cuenta de desarrollador de Google Play tiene un costo único de inscripción (no recurrente); no es gratuita, pero tampoco es un costo de infraestructura continuo. Confirmar este costo como excepción al presupuesto $0 antes de planear el lanzamiento.*

---

## 3. Metodología de trabajo: Scrum simplificado
Dado que el equipo es de 1 a 2 personas, se adapta Scrum a una versión ligera, manteniendo los artefactos que aportan valor real y descartando ceremonias pensadas para equipos grandes.

### 3.1 Artefactos
*   **Backlog:** GitHub Projects (tablero Kanban integrado al repositorio, sin costo).
*   **Historias de usuario:** Definidas en este documento como punto de partida; nuevas historias se agregan directamente al backlog de GitHub.
*   **Sprints:** Duración de 1 semana. Permite ver progreso tangible en un proyecto part-time y ajustar el rumbo rápido.

### 3.2 Flujo de trabajo

*   Cada historia de usuario debe tener criterios de aceptación claros antes de pasar a “To Do”.
*   **Cierre de sprint:** Revisión rápida de qué se completó y qué se mueve al siguiente (reemplaza la retro formal por una nota corta de 5 minutos).

---

## 4. Alcance del MVP
El MVP se centra en un modo de juego: **grid aleatorio (no diario)**. El modo “desafío diario” estilo Wordle queda definido como mejora post-MVP.

### 4.1 Dentro del alcance (MVP)
*   Generación de un tablero 3x3 con categorías aleatorias válidas (que garanticen al menos una respuesta posible por celda).
*   Búsqueda/autocompletado de jugador profesional al responder una celda.
*   Validación de respuesta contra el backend.
*   Feedback visual de acierto/error por celda.
*   Pantalla de resultado final (tablero completo, aciertos/errores).
*   Base de datos inicial poblada con jugadores y equipos profesionales reales.

### 4.2 Fuera del alcance (futuro)
*   Modo diario / desafío compartido (estilo Wordle).
*   Cuentas de usuario, login, perfiles.
*   Estadísticas históricas, rachas, leaderboard.
*   Publicidad / monetización.
*   Versión web (React) — la arquitectura la deja preparada, pero no se construye en el MVP.

---

## 5. Épicas e historias de usuario
*Prioridad: Alta = bloqueante para jugar el MVP. Media = mejora la experiencia pero no bloquea. Baja = deseable, puede moverse a un sprint posterior.*

### Épica 1 — Datos: categorías y jugadores
Cubre la definición del modelo de datos y la población inicial con jugadores, equipos y categorías reales de Valorant.

| ID | Historia de usuario | Criterios de aceptación | Prioridad |
| :--- | :--- | :--- | :--- |
| **E1-1** | Como desarrollador, quiero definir el modelo de datos relacional (jugadores, equipos, regiones, agentes, roles, torneos) para poder generar tableros y validar respuestas. | - Existe un diagrama o script SQL con tablas y relaciones (jugador, equipo, región, agente, rol).<br>- Las relaciones permiten responder consultas de intersección (ej. jugador de equipo X y región Y).<br>- Se definen índices para las columnas usadas en los JOINs de intersección. | **Alta** |
| **E1-2** | Como desarrollador, quiero poblar la base de datos con un set inicial de jugadores profesionales y sus equipos/regiones/agentes para tener contenido jugable. | - Existe un script de carga (seed) reproducible.<br>- El set inicial cubre al menos las regiones principales (Américas, EMEA, Pacífico, China).<br>- Cada jugador tiene asociado al menos un equipo y una región. | **Alta** |
| **E1-3** | Como desarrollador, quiero definir qué combinaciones de categorías son válidas para garantizar que todo tablero generado tenga al menos una respuesta posible por celda. | - Existe una regla o validación que descarta combinaciones de fila/columna sin ningún jugador que las cumpla.<br>- Se documenta la lista de tipos de categoría soportados en el MVP (equipo, región, rol; agente queda documentado como posible extensión). | **Alta** |

### Épica 2 — Backend (API en Go)
Cubre los endpoints necesarios para generar tableros y validar respuestas.

| ID | Historia de usuario | Criterios de aceptación | Prioridad |
| :--- | :--- | :--- | :--- |
| **E2-1** | Como jugador, quiero solicitar un nuevo tablero aleatorio para poder iniciar una partida. | - Existe un endpoint (ej. `POST /games`) que devuelve 9 categorías (3 filas, 3 columnas) válidas.<br>- Cada solicitud genera una combinación distinta dentro de lo razonable (no siempre el mismo tablero). | **Alta** |
| **E2-2** | Como jugador, quiero enviar el nombre de un jugador profesional para una celda y recibir si es correcto o no. | - Existe un endpoint (ej. `POST /games/{id}/guess`) que recibe celda + nombre de jugador.<br>- La respuesta indica acierto/error y, en caso de acierto, devuelve datos básicos del jugador (equipo, foto si aplica).<br>- Se evita que el cliente pueda inferir la respuesta correcta sin haber acertado (no exponer la lista completa de válidos). | **Alta** |
| **E2-3** | Como jugador, quiero que al escribir un nombre se sugieran coincidencias para evitar errores de tipeo. | - Existe un endpoint de búsqueda (ej. `GET /players?q=`) que devuelve開 coincidencias parciales.<br>- La búsqueda no es sensible a mayúsculas/acentos. | **Media** |

### Épica 3 — App móvil (React Native)

| ID | Historia de usuario | Criterios de aceptación | Prioridad |
| :--- | :--- | :--- | :--- |
| **E3-1** | Como jugador, quiero ver un tablero 3x3 con las categorías de cada fila y columna para entender qué debo responder en cada celda. | - El tablero se renderiza con las 3 categorías de fila y 3 de columna visibles.<br>- El diseño es legible en pantallas de móvil estándar (no requiere zoom). | **Alta** |
| **E3-2** | Como jugador, quiero tocar una celda y escribir/seleccionar un jugador para responder esa intersección. | - Al tocar una celda vacía se abre un input con autocompletado conectado al backend.<br>- Seleccionar una sugerencia envía la respuesta automáticamente. | **Alta** |
| **E3-3** | Como jugador, quiero ver feedback inmediato (correcto/incorrecto) al responder una celda. | - La celda cambia de color o estado visual según el resultado.<br>- Si es correcto, se muestra el nombre/equipo del jugador en la celda. | **Alta** |
| **E3-4** | Como jugador, quiero ver un resumen final al completar o fallar el tablero para saber mi resultado. | - Al llenar las 9 celdas (o agotar intentos, si se define un límite) se muestra una pantalla de resumen.<br>- El resumen permite iniciar una nueva partida. | **Media** |

### Épica 4 — Monetización (post-MVP)
*Se aborda después de validar que el juego es divertido y retiene usuarios. Incluir anuncios prematuramente, sin tráfico, no genera ingresos significativos y puede dañar la primera impresión del producto.*

| ID | Historia de usuario | Criterios de aceptación | Prioridad |
| :--- | :--- | :--- | :--- |
| **E4-1** | Como desarrollador, quiero integrar un proveedor de anuncios (ej. Google AdMob) para mostrar publicidad sin costo de implementación. | - Se evalúa AdMob como opción principal por ser gratuito de integrar y de uso extendido en apps de Google Play.<br>- Los anuncios no bloquean la jugabilidad central (ej. banner o intersticial entre partidas, no durante el llenado del tablero). | **Baja** |

---

## 6. Riesgos y supuestos iniciales
*   **Riesgo de datos:** Mantener actualizada la base de jugadores/equipos (transferencias, retiros) requiere mantenimiento manual o un proceso de actualización periódico. No se resuelve en el MVP.
*   **Riesgo de alcance:** Agregar el modo diario o cuentas de usuario antes de validar el modo aleatorio puede retrasar el lanzamiento sin necesidad.
*   **Supuesto:** El tier gratuito de la base de datos y el backend (Supabase/Neon, Render/Fly.io) es suficiente para el volumen de tráfico inicial. Debe revalidarse si el proyecto gana tracción.
*   **Supuesto:** La cuenta de desarrollador de Google Play implica un costo único de inscripción, fuera del alcance de “sin presupuesto” estricto; debe confirmarse y presupuestarse aparte antes del lanzamiento a producción.

---

## 7. Próximos pasos sugeridos
1.  Validar y ajustar este documento con tu compañero de equipo.
2.  Crear el repositorio en GitHub y el tablero de GitHub Projects con estas historias como columnas iniciales.
3.  Definir el modelo de datos detallado (diagrama entidad-relación) — Épica 1.
4.  Decidir el proveedor gratuito de Postgres (Supabase vs Neon) tras comparar límites actuales del tier gratuito.
5.  Iniciar Sprint 1 enfocado únicamente en la Épica 1 (datos), que es la base de todo lo demás.