# Troubleshooting: the wheel on Projects runs against your fingers

On Projects, the wheel over the list chooses the next project as your fingers move down. The
dashboard follows your Mac's scroll setting to do that. This page covers a list that chooses the
project above when your fingers move down.

## Why the list runs backwards

| If | Then |
|---|---|
| You changed the scroll setting in System Settings with the dashboard open | Quit the dashboard and open it again. The dashboard reads the setting only as it opens. |
| You reach the Mac over SSH from a device that scrolls the other way | The list runs backwards, because the dashboard follows the Mac's setting, not the device's. For example, a Mac set to traditional scrolling, reached from an iPad or an iPhone. |
| You run a scroll-reversing tool, such as Scroll Reverser, Mos or LinearMouse | The list can run backwards. The tool gives the wheel a direction the setting doesn't show. |
| Your terminal inverts the wheel | The list can run backwards, for the same reason. |

Everywhere else, the wheel scrolls what's under the pointer, and macOS scrolls it as the setting
says. Only the list on Projects reads the setting itself.

## Choosing a project with the keys

In the list, `↑` and `↓` choose a project whichever way the wheel runs:

- `↑` chooses the project above.
- `↓` chooses the project below.

## Related

- [Projects](../dashboard/projects.md): the list of projects, the sessions of each project, and
  the keys on Projects.
- [Every page](../dashboard/every-page.md): what scrolls on each page.
