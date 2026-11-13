import { displayName, type User } from "@/lib/types";
import { initials } from "@/lib/format";

/** Avatar shows a user's picture, falling back to their initials. */
export default function Avatar({
  user,
  size = "md",
}: {
  user: User | null;
  size?: "md" | "lg" | "xl";
}) {
  const className =
    size === "xl" ? "avatar avatar-xl" : size === "lg" ? "avatar avatar-lg" : "avatar";

  if (!user) {
    return (
      <span className={className} aria-hidden="true">
        ?
      </span>
    );
  }

  if (user.avatar_url) {
    return (
      // A plain <img> keeps remote avatars working without configuring a list of
      // allowed hosts, and avoids an optimiser round trip for a 32px image.
      // no-referrer means the image host learns the reader's address but not which
      // page they were reading. Instances that care can turn remote avatars off
      // entirely (KNOT_ALLOW_REMOTE_AVATARS).
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={user.avatar_url}
        alt=""
        className={className}
        loading="lazy"
        referrerPolicy="no-referrer"
        width={96}
        height={96}
      />
    );
  }

  return (
    <span className={className} aria-hidden="true">
      {initials(displayName(user))}
    </span>
  );
}
