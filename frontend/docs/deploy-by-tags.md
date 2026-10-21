# Автоматическая публикация сайта по Git-тегам

Персональный сайт собирается GitHub Actions и публикуется через публичные GitHub Releases. Сервер `178.212.15.69` самостоятельно проверяет наличие нового релиза и забирает архив. **На сервере не нужны Node.js, pnpm, GitHub-токен или SSH-доступ GitHub Actions.**

## Новый релиз

После внесения изменений в `master` создайте новый тег в формате `vMAJOR.MINOR.PATCH`, например `v0.1.1`, и отправьте его в GitHub. Можно создать тег через интерфейс GitHub либо выполнить:

```bash
git switch master
git pull --ff-only
git tag v0.1.1
git push origin v0.1.1
```

Вручную собирать сайт, подключаться по SSH и загружать файлы **не требуется**.

1. `.github/workflows/release.yml` запускается только при публикации тега `v*`.
2. Проверяется SemVer и принадлежность коммита ветке `master`.
3. Запускаются TypeScript, ESLint, Next.js static export, проверка ссылок и адаптивности.
4. Только после успеха создаётся GitHub Release с `site.tar.gz` и SHA-256 digest.
5. `personal-site-deploy.timer` на сервере опрашивает GitHub примерно раз в 2 минуты, сверяет SHA-256, распаковывает отдельную версию и атомарно обновляет `current`.
6. После переключения проверяются страницы через локальный Nginx/HTTPS. При ошибке автоматически возвращается предыдущая версия.

Изменения в `master` **без нового тега** не публикуются. Повторное использование тега не требуется; версии должны увеличиваться. Текущий релиз доступен по адресу `https://kuminovvv.com`.

## Диагностика

- GitHub: `https://github.com/Kuminovvv/personal-site/actions` и вкладка Releases.
- Сервер: `systemctl status personal-site-deploy.timer`.
- Лог: `journalctl -u personal-site-deploy.service -n 100 --no-pager`.
- Текущая версия: `cat /var/www/kuminovvv.com/.deployed-tag`.
- Активный каталог: `readlink -f /var/www/kuminovvv.com/current`.

Деплой работает с правами `site-deploy`, без root/SSH-ключей в GitHub. Предыдущие каталоги остаются в `releases/` для восстановления. Nginx и конфигурация Let's Encrypt при релизах не изменяются. Исходники Backend не задействованы.
